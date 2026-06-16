package tracker

import (
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"openpt/internal/clientemu"
)

type Client struct {
	http *http.Client
	log  *slog.Logger
}

func New(timeout time.Duration, proxy string, log *slog.Logger) (*Client, error) {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	if proxy != "" {
		u, err := url.Parse(proxy)
		if err != nil {
			return nil, err
		}
		tr.Proxy = http.ProxyURL(u)
	}
	return &Client{http: &http.Client{Timeout: timeout, Transport: tr}, log: log}, nil
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
	for _, h := range headers {
		req.Header.Add(h.Name, h.Value)
	}
	c.log.Info("announce request", "host", req.URL.Host, "scheme", req.URL.Scheme)
	resp, err := c.http.Do(req)
	if err != nil {
		return Response{}, err
	}
	defer resp.Body.Close()
	body, err := decodedBody(resp)
	if err != nil {
		return Response{}, err
	}
	data, err := io.ReadAll(body)
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

func decodedBody(resp *http.Response) (io.Reader, error) {
	switch strings.ToLower(resp.Header.Get("Content-Encoding")) {
	case "gzip":
		return gzip.NewReader(resp.Body)
	case "deflate":
		zr, err := zlib.NewReader(resp.Body)
		if err == nil {
			return zr, nil
		}
		return flate.NewReader(resp.Body), nil
	default:
		return resp.Body, nil
	}
}
