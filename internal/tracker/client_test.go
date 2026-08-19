package tracker

import (
	"bytes"
	"compress/flate"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAnnounceRejectsOversizedTrackerResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, strings.Repeat("x", maxTrackerResponseBytes+1))
	}))
	defer server.Close()

	c, err := New(Options{
		Timeout:             time.Second,
		ReuseConnections:    true,
		MaxIdleConns:        10,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     time.Second,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}

	_, err = c.Announce(context.Background(), server.URL, "", nil)
	if err == nil || !strings.Contains(err.Error(), "tracker response exceeds size limit") {
		t.Fatalf("announce error = %v, want size limit error", err)
	}
}

func TestAnnounceUDP(t *testing.T) {
	listener, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	_ = listener.SetDeadline(time.Now().Add(3 * time.Second))

	serverErr := make(chan error, 1)
	go func() {
		buf := make([]byte, 2048)
		n, addr, err := listener.ReadFromUDP(buf)
		if err != nil {
			serverErr <- err
			return
		}
		if n != 16 || binary.BigEndian.Uint64(buf[0:8]) != udpProtocolID || binary.BigEndian.Uint32(buf[8:12]) != 0 {
			serverErr <- fmt.Errorf("invalid connect request: %x", buf[:n])
			return
		}
		connectResponse := make([]byte, 16)
		binary.BigEndian.PutUint32(connectResponse[0:4], 0)
		copy(connectResponse[4:8], buf[12:16])
		binary.BigEndian.PutUint64(connectResponse[8:16], 0x1020304050607080)
		if _, err := listener.WriteToUDP(connectResponse, addr); err != nil {
			serverErr <- err
			return
		}

		n, addr, err = listener.ReadFromUDP(buf)
		if err != nil {
			serverErr <- err
			return
		}
		if n != 98 || binary.BigEndian.Uint32(buf[8:12]) != 1 || string(buf[16:36]) != "abcdefghijklmnopqrst" || string(buf[36:56]) != "-AA0000-abcdefghijkl" {
			serverErr <- fmt.Errorf("invalid announce request: %x", buf[:n])
			return
		}
		if binary.BigEndian.Uint64(buf[72:80]) != 123 || binary.BigEndian.Uint32(buf[80:84]) != 2 || binary.BigEndian.Uint16(buf[96:98]) != 51413 {
			serverErr <- fmt.Errorf("invalid announce fields: %x", buf[:n])
			return
		}
		announceResponse := make([]byte, 20)
		binary.BigEndian.PutUint32(announceResponse[0:4], 1)
		copy(announceResponse[4:8], buf[12:16])
		binary.BigEndian.PutUint32(announceResponse[8:12], 1800)
		binary.BigEndian.PutUint32(announceResponse[12:16], 4)
		binary.BigEndian.PutUint32(announceResponse[16:20], 9)
		_, err = listener.WriteToUDP(announceResponse, addr)
		serverErr <- err
	}()

	c := newTestTrackerClient(t)
	query := "info_hash=abcdefghijklmnopqrst&peer_id=-AA0000-abcdefghijkl&port=51413&uploaded=123&downloaded=0&left=0&event=started&numwant=25&key=abcdef01"
	resp, err := c.Announce(context.Background(), "udp://"+listener.LocalAddr().String()+"/announce", query, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := <-serverErr; err != nil {
		t.Fatal(err)
	}
	if resp.Interval != 1800 || resp.Seeders != 9 || resp.Leechers != 4 {
		t.Fatalf("response = %+v", resp)
	}
}

func TestAnnounceDecodesRawDeflateResponse(t *testing.T) {
	var compressed bytes.Buffer
	fw, err := flate.NewWriter(&compressed, flate.DefaultCompression)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = fw.Write([]byte("d8:intervali1800e8:completei2e10:incompletei1ee"))
	if err := fw.Close(); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Encoding", "deflate")
		_, _ = w.Write(compressed.Bytes())
	}))
	defer server.Close()

	c := newTestTrackerClient(t)
	resp, err := c.Announce(context.Background(), server.URL, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Interval != 1800 || resp.Seeders != 1 || resp.Leechers != 1 {
		t.Fatalf("response = %+v, want interval=1800 seeders=1 leechers=1", resp)
	}
}

func TestAnnounceAppendsQueryBeforeFragment(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("info_hash"); got != "abc" {
			t.Fatalf("info_hash query = %q, want abc; raw query=%q", got, r.URL.RawQuery)
		}
		_, _ = io.WriteString(w, "d8:intervali1800e8:completei1e10:incompletei0ee")
	}))
	defer server.Close()

	c := newTestTrackerClient(t)
	if _, err := c.Announce(context.Background(), server.URL+"/announce#ignored", "info_hash=abc&peer_id=peer", nil); err != nil {
		t.Fatal(err)
	}
}

func TestParseUDPQueryPreservesPlus(t *testing.T) {
	raw := "info_hash=%00%01%2B%20abc&peer_id=-AA0000-123+456%2B789&port=6881"
	vals, err := parseUDPQuery(raw)
	if err != nil {
		t.Fatal(err)
	}
	// %2B -> '+', %20 -> ' ', and '+' remains '+'
	if vals["info_hash"] != "\x00\x01+ abc" {
		t.Fatalf("info_hash = %q, want %q", vals["info_hash"], "\x00\x01+ abc")
	}
	if vals["peer_id"] != "-AA0000-123+456+789" {
		t.Fatalf("peer_id = %q, want %q", vals["peer_id"], "-AA0000-123+456+789")
	}
	if vals["port"] != "6881" {
		t.Fatalf("port = %q, want 6881", vals["port"])
	}
}

// TestAnnounceUDPRejectedWhenProxyConfigured 验证配置 tracker.proxy 后 UDP announce
// 立即返回明确错误而不发起网络请求，调度器据此切换到其它 tracker。
func TestAnnounceUDPRejectedWhenProxyConfigured(t *testing.T) {
	c, err := New(Options{
		Timeout:             time.Second,
		Proxy:               "http://127.0.0.1:7890",
		ReuseConnections:    true,
		MaxIdleConns:        10,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     time.Second,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Announce(context.Background(), "udp://127.0.0.1:9/announce", "info_hash=abc", nil)
	if err == nil || !strings.Contains(err.Error(), "unavailable when tracker.proxy is configured") {
		t.Fatalf("announce error = %v, want proxy rejection", err)
	}
}

// TestAnnounceUDPTimesOut 验证无响应的 UDP tracker 会话受 timeout 约束而结束，
// 与 HTTP 路径的 http.Client.Timeout 语义一致（而非无限阻塞）。
func TestAnnounceUDPTimesOut(t *testing.T) {
	// 绑定一个从不响应的 UDP socket，模拟 tracker 失效。
	listener, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	c, err := New(Options{
		Timeout:             200 * time.Millisecond,
		ReuseConnections:    true,
		MaxIdleConns:        10,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     time.Second,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	_, err = c.Announce(context.Background(), "udp://"+listener.LocalAddr().String()+"/announce", "info_hash=abc", nil)
	if err == nil {
		t.Fatal("expected UDP announce to time out, got success")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("UDP time out took %v, want bounded by tracker timeout", elapsed)
	}
}

func newTestTrackerClient(t *testing.T) *Client {
	t.Helper()
	c, err := New(Options{
		Timeout:             time.Second,
		ReuseConnections:    true,
		MaxIdleConns:        10,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     time.Second,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	return c
}
