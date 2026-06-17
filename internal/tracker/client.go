package tracker

import (
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"openpt/internal/clientemu"
)

const maxTrackerResponseBytes = 1 << 20

type Options struct {
	Timeout             time.Duration
	Proxy               string
	ReuseConnections    bool
	MaxIdleConns        int
	MaxIdleConnsPerHost int
	IdleConnTimeout     time.Duration
}

type Client struct {
	mu                 sync.RWMutex
	http               *http.Client
	reuseConnections   bool
	closeIdleTransport func()
	log                *slog.Logger
}

func New(opts Options, log *slog.Logger) (*Client, error) {
	httpClient, closeIdle, err := newHTTPClient(opts)
	if err != nil {
		return nil, err
	}
	return &Client{
		http:               httpClient,
		reuseConnections:   opts.ReuseConnections,
		closeIdleTransport: closeIdle,
		log:                log,
	}, nil
}

func (c *Client) Configure(opts Options) error {
	httpClient, closeIdle, err := newHTTPClient(opts)
	if err != nil {
		return err
	}
	c.mu.Lock()
	oldCloseIdle := c.closeIdleTransport
	c.http = httpClient
	c.reuseConnections = opts.ReuseConnections
	c.closeIdleTransport = closeIdle
	c.mu.Unlock()
	if oldCloseIdle != nil {
		oldCloseIdle()
	}
	return nil
}

func newHTTPClient(opts Options) (*http.Client, func(), error) {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.MaxIdleConns = opts.MaxIdleConns
	tr.MaxIdleConnsPerHost = opts.MaxIdleConnsPerHost
	tr.IdleConnTimeout = opts.IdleConnTimeout
	if opts.Proxy != "" {
		u, err := url.Parse(opts.Proxy)
		if err != nil {
			return nil, nil, err
		}
		tr.Proxy = http.ProxyURL(u)
	}
	return &http.Client{Timeout: opts.Timeout, Transport: tr}, tr.CloseIdleConnections, nil
}

func (c *Client) Announce(ctx context.Context, baseURL, query string, headers []clientemu.Header) (Response, error) {
	sep := "?"
	if strings.Contains(baseURL, "?") {
		sep = "&"
	}
	full := baseURL + sep + query
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, full, nil)
	if err != nil {
		return Response{}, err
	}
	req.Host = req.URL.Host
	c.mu.RLock()
	httpClient := c.http
	reuseConnections := c.reuseConnections
	c.mu.RUnlock()
	for _, h := range headers {
		if reuseConnections && strings.EqualFold(h.Name, "Connection") && strings.EqualFold(h.Value, "close") {
			continue
		}
		req.Header.Add(h.Name, h.Value)
	}
	c.log.Info("announce request", "host", req.URL.Host, "scheme", req.URL.Scheme)
	resp, err := httpClient.Do(req)
	if err != nil {
		return Response{}, err
	}
	body, err := decodedBody(resp)
	if err != nil {
		_ = resp.Body.Close()
		return Response{}, err
	}
	defer body.Close()
	data, err := readLimited(body, maxTrackerResponseBytes)
	if err != nil {
		return Response{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Response{}, fmt.Errorf("tracker returned HTTP %d: %s", resp.StatusCode, string(data))
	}
	parsed, err := ParseResponse(data)
	if err != nil {
		return Response{}, err
	}
	if parsed.Failure != "" {
		return Response{}, fmt.Errorf("tracker failure: %s", parsed.Failure)
	}
	return parsed, nil
}

func decodedBody(resp *http.Response) (io.ReadCloser, error) {
	switch strings.ToLower(resp.Header.Get("Content-Encoding")) {
	case "gzip":
		gr, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, err
		}
		return closeBoth{ReadCloser: gr, other: resp.Body}, nil
	case "deflate":
		zr, err := zlib.NewReader(resp.Body)
		if err == nil {
			return closeBoth{ReadCloser: zr, other: resp.Body}, nil
		}
		return closeBoth{ReadCloser: flate.NewReader(resp.Body), other: resp.Body}, nil
	default:
		return resp.Body, nil
	}
}

type closeBoth struct {
	io.ReadCloser
	other io.Closer
}

func (c closeBoth) Close() error {
	err := c.ReadCloser.Close()
	if otherErr := c.other.Close(); err == nil {
		err = otherErr
	}
	return err
}

func readLimited(r io.Reader, maxBytes int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, errors.New("tracker response exceeds size limit")
	}
	return data, nil
}
