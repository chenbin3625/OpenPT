package web

import (
	"encoding/json"
	_ "embed"
	"fmt"
	"net/http"
	"time"

	"openpt/internal/bandwidth"
	"openpt/internal/scheduler"
	"openpt/internal/store"
)

//go:embed index.html
var indexHTML []byte

// StatusResponse represents the full status response.
type StatusResponse struct {
	Torrents []scheduler.TorrentStatus `json:"torrents"`
}

// Handler provides HTTP handlers for the web UI.
type Handler struct {
	store     *store.Store
	scheduler *scheduler.Scheduler
	bw        *bandwidth.Dispatcher
}

// New creates a new web Handler.
func New(st *store.Store, s *scheduler.Scheduler, bw *bandwidth.Dispatcher) *Handler {
	return &Handler{
		store:     st,
		scheduler: s,
		bw:        bw,
	}
}

// RegisterRoutes registers the web UI routes on the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/", h.handleIndex)
	mux.HandleFunc("/api/status", h.handleStatus)
	mux.HandleFunc("/api/events", h.handleEvents)
}

func (h *Handler) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(indexHTML)
}

func (h *Handler) handleStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	resp := StatusResponse{
		Torrents: h.scheduler.Status(),
	}
	json.NewEncoder(w).Encode(resp)
}

func (h *Handler) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	// Send initial status
	h.sendStatus(w, flusher)

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			h.sendStatus(w, flusher)
		}
	}
}

func (h *Handler) sendStatus(w http.ResponseWriter, flusher http.Flusher) {
	resp := StatusResponse{
		Torrents: h.scheduler.Status(),
	}
	data, err := json.Marshal(resp)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "data: %s\n\n", data)
	flusher.Flush()
}
