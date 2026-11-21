package cockpit

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"sync"
	"time"

	"github.com/alexandrmotologa/flakeprobe/pkg/report"
)

//go:embed web/*
var webFS embed.FS

// Server hosts the embedded interactive cockpit dashboard.
type Server struct {
	Port       int
	report     *report.FullReport
	reportMu   sync.RWMutex
	httpServer *http.Server
	clients    map[chan string]bool
	clientsMu  sync.Mutex
}

// NewServer creates a new cockpit server instance.
func NewServer(port int, initialReport *report.FullReport) *Server {
	return &Server{
		Port:    port,
		report:  initialReport,
		clients: make(map[chan string]bool),
	}
}

// UpdateReport safely updates the active report shown in the dashboard.
func (s *Server) UpdateReport(rep *report.FullReport) {
	s.reportMu.Lock()
	s.report = rep
	s.reportMu.Unlock()

	data, err := json.Marshal(map[string]interface{}{
		"type":    "report_update",
		"payload": rep,
	})
	if err == nil {
		s.broadcast(string(data))
	}
}

func (s *Server) broadcast(msg string) {
	s.clientsMu.Lock()
	defer s.clientsMu.Unlock()

	for ch := range s.clients {
		select {
		case ch <- msg:
		default:
		}
	}
}

// Start launches the cockpit web server.
func (s *Server) Start() error {
	mux := http.NewServeMux()

	// Static web assets
	webSubFS, err := fs.Sub(webFS, "web")
	if err != nil {
		return fmt.Errorf("failed to open embedded web filesystem: %w", err)
	}

	mux.Handle("/", http.FileServer(http.FS(webSubFS)))
	mux.HandleFunc("/api/report", s.handleReport)
	mux.HandleFunc("/api/stream", s.handleStream)

	s.httpServer = &http.Server{
		Addr:    fmt.Sprintf(":%d", s.Port),
		Handler: mux,
	}

	return s.httpServer.ListenAndServe()
}

// Stop terminates the cockpit server.
func (s *Server) Stop(ctx context.Context) error {
	if s.httpServer != nil {
		return s.httpServer.Shutdown(ctx)
	}
	return nil
}

func (s *Server) handleReport(w http.ResponseWriter, r *http.Request) {
	s.reportMu.RLock()
	rep := s.report
	s.reportMu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	if rep == nil {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error": "No report loaded yet",
		})
		return
	}

	_ = json.NewEncoder(w).Encode(rep)
}

func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	msgCh := make(chan string, 16)
	s.clientsMu.Lock()
	s.clients[msgCh] = true
	s.clientsMu.Unlock()

	defer func() {
		s.clientsMu.Lock()
		delete(s.clients, msgCh)
		close(msgCh)
		s.clientsMu.Unlock()
	}()

	notify := r.Context().Done()

	for {
		select {
		case <-notify:
			return
		case msg := <-msgCh:
			fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		case <-time.After(15 * time.Second):
			// Keepalive ping
			fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}
