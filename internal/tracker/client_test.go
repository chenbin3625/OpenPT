package tracker

import (
	"bytes"
	"compress/flate"
	"context"
	"io"
	"log/slog"
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
