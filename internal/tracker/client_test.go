package tracker

import (
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
