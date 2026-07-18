package tracker

import (
	"bufio"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
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
	timeout            time.Duration
	proxy              string
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
		timeout:            opts.Timeout,
		proxy:              opts.Proxy,
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
	c.timeout = opts.Timeout
	c.proxy = opts.Proxy
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
	tr.DisableKeepAlives = !opts.ReuseConnections
	return &http.Client{Timeout: opts.Timeout, Transport: tr}, tr.CloseIdleConnections, nil
}

func (c *Client) Announce(ctx context.Context, baseURL, query string, headers []clientemu.Header) (Response, error) {
	parsedBase, err := url.Parse(baseURL)
	if err != nil {
		return Response{}, err
	}
	if strings.EqualFold(parsedBase.Scheme, "udp") {
		return c.announceUDP(ctx, parsedBase, query)
	}
	full, err := appendRawQuery(baseURL, query)
	if err != nil {
		return Response{}, err
	}
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
		// net/http 会忽略 Header 中的 Host，必须通过 req.Host 设置
		if strings.EqualFold(h.Name, "Host") {
			req.Host = h.Value
			continue
		}
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

const udpProtocolID uint64 = 0x41727101980

func (c *Client) announceUDP(ctx context.Context, trackerURL *url.URL, rawQuery string) (Response, error) {
	c.mu.RLock()
	timeout := c.timeout
	proxy := c.proxy
	c.mu.RUnlock()
	if proxy != "" {
		return Response{}, errors.New("UDP tracker announce is unavailable when tracker.proxy is configured")
	}
	if trackerURL.Host == "" {
		return Response{}, errors.New("UDP tracker URL has no host")
	}

	conn, err := (&net.Dialer{}).DialContext(ctx, "udp", trackerURL.Host)
	if err != nil {
		return Response{}, err
	}
	defer conn.Close()
	stopClose := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stopClose()
	if timeout > 0 {
		_ = conn.SetDeadline(time.Now().Add(timeout))
	}

	connectTx, err := randomUint32()
	if err != nil {
		return Response{}, err
	}
	connectRequest := make([]byte, 16)
	binary.BigEndian.PutUint64(connectRequest[0:8], udpProtocolID)
	binary.BigEndian.PutUint32(connectRequest[8:12], 0)
	binary.BigEndian.PutUint32(connectRequest[12:16], connectTx)
	if _, err := conn.Write(connectRequest); err != nil {
		return Response{}, err
	}
	connectResponse := make([]byte, 2048)
	n, err := conn.Read(connectResponse)
	if err != nil {
		return Response{}, err
	}
	connectResponse = connectResponse[:n]
	if err := validateUDPResponse(connectResponse, 0, connectTx, 16); err != nil {
		return Response{}, fmt.Errorf("UDP connect: %w", err)
	}
	connectionID := binary.BigEndian.Uint64(connectResponse[8:16])

	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		return Response{}, fmt.Errorf("parse UDP announce query: %w", err)
	}
	for key, vals := range trackerURL.Query() {
		if _, exists := values[key]; !exists {
			values[key] = vals
		}
	}
	request, tx, err := buildUDPAnnounceRequest(connectionID, values)
	if err != nil {
		return Response{}, err
	}
	if _, err := conn.Write(request); err != nil {
		return Response{}, err
	}
	announceResponse := make([]byte, 64*1024)
	n, err = conn.Read(announceResponse)
	if err != nil {
		return Response{}, err
	}
	announceResponse = announceResponse[:n]
	if err := validateUDPResponse(announceResponse, 1, tx, 20); err != nil {
		return Response{}, fmt.Errorf("UDP announce: %w", err)
	}
	return Response{
		Interval: int(binary.BigEndian.Uint32(announceResponse[8:12])),
		Leechers: int(binary.BigEndian.Uint32(announceResponse[12:16])),
		Seeders:  int(binary.BigEndian.Uint32(announceResponse[16:20])),
	}, nil
}

func buildUDPAnnounceRequest(connectionID uint64, values url.Values) ([]byte, uint32, error) {
	infoHash := []byte(values.Get("info_hash"))
	peerID := []byte(values.Get("peer_id"))
	if len(infoHash) != 20 {
		return nil, 0, fmt.Errorf("UDP info_hash must be 20 bytes, got %d", len(infoHash))
	}
	if len(peerID) != 20 {
		return nil, 0, fmt.Errorf("UDP peer_id must be 20 bytes, got %d", len(peerID))
	}
	downloaded, err := queryInt64(values, "downloaded")
	if err != nil {
		return nil, 0, err
	}
	left, err := queryInt64(values, "left")
	if err != nil {
		return nil, 0, err
	}
	uploaded, err := queryInt64(values, "uploaded")
	if err != nil {
		return nil, 0, err
	}
	port, err := strconv.ParseUint(values.Get("port"), 10, 16)
	if err != nil || port == 0 {
		return nil, 0, errors.New("UDP port must be in 1..65535")
	}
	numwant := int64(-1)
	if text := values.Get("numwant"); text != "" {
		numwant, err = strconv.ParseInt(text, 10, 32)
		if err != nil {
			return nil, 0, fmt.Errorf("invalid UDP numwant: %w", err)
		}
	}
	key := uint64(0)
	if text := values.Get("key"); text != "" {
		key, err = strconv.ParseUint(text, 16, 32)
		if err != nil {
			return nil, 0, fmt.Errorf("invalid UDP key: %w", err)
		}
	}
	tx, err := randomUint32()
	if err != nil {
		return nil, 0, err
	}
	request := make([]byte, 98)
	binary.BigEndian.PutUint64(request[0:8], connectionID)
	binary.BigEndian.PutUint32(request[8:12], 1)
	binary.BigEndian.PutUint32(request[12:16], tx)
	copy(request[16:36], infoHash)
	copy(request[36:56], peerID)
	binary.BigEndian.PutUint64(request[56:64], uint64(downloaded))
	binary.BigEndian.PutUint64(request[64:72], uint64(left))
	binary.BigEndian.PutUint64(request[72:80], uint64(uploaded))
	binary.BigEndian.PutUint32(request[80:84], udpEvent(values.Get("event")))
	if ip := net.ParseIP(values.Get("ip")).To4(); ip != nil {
		copy(request[84:88], ip)
	}
	binary.BigEndian.PutUint32(request[88:92], uint32(key))
	binary.BigEndian.PutUint32(request[92:96], uint32(int32(numwant)))
	binary.BigEndian.PutUint16(request[96:98], uint16(port))
	return request, tx, nil
}

func queryInt64(values url.Values, key string) (int64, error) {
	n, err := strconv.ParseInt(values.Get(key), 10, 64)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("UDP %s must be a non-negative integer", key)
	}
	return n, nil
}

func udpEvent(event string) uint32 {
	switch event {
	case "completed":
		return 1
	case "started":
		return 2
	case "stopped":
		return 3
	default:
		return 0
	}
}

func validateUDPResponse(data []byte, expectedAction, transactionID uint32, minLength int) error {
	if len(data) < 8 {
		return errors.New("response is too short")
	}
	action := binary.BigEndian.Uint32(data[0:4])
	gotTransactionID := binary.BigEndian.Uint32(data[4:8])
	if gotTransactionID != transactionID {
		return errors.New("transaction ID mismatch")
	}
	if action == 3 {
		return fmt.Errorf("tracker failure: %s", string(data[8:]))
	}
	if action != expectedAction {
		return fmt.Errorf("unexpected action %d", action)
	}
	if len(data) < minLength {
		return fmt.Errorf("response is too short: got %d bytes, want at least %d", len(data), minLength)
	}
	return nil
}

func randomUint32() (uint32, error) {
	var data [4]byte
	if _, err := rand.Read(data[:]); err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint32(data[:]), nil
}

func appendRawQuery(baseURL, query string) (string, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	if query == "" {
		return u.String(), nil
	}
	if u.RawQuery == "" {
		u.RawQuery = query
	} else {
		u.RawQuery += "&" + query
	}
	return u.String(), nil
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
		br := bufio.NewReader(resp.Body)
		if looksLikeZlib(br) {
			zr, err := zlib.NewReader(br)
			if err != nil {
				return nil, err
			}
			return closeBoth{ReadCloser: zr, other: resp.Body}, nil
		}
		return closeBoth{ReadCloser: flate.NewReader(br), other: resp.Body}, nil
	default:
		return resp.Body, nil
	}
}

func looksLikeZlib(br *bufio.Reader) bool {
	header, err := br.Peek(2)
	if err != nil {
		return false
	}
	cmf, flg := int(header[0]), int(header[1])
	return cmf&0x0f == 8 && ((cmf<<8)+flg)%31 == 0
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
