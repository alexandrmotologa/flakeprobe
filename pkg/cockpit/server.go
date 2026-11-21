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

	"github.com/alexandrmotologa/flakeprobe/pkg/diff"
	"github.com/alexandrmotologa/flakeprobe/pkg/entropy"
	"github.com/alexandrmotologa/flakeprobe/pkg/mockagent"
	"github.com/alexandrmotologa/flakeprobe/pkg/report"
	"github.com/alexandrmotologa/flakeprobe/pkg/runner"
	"github.com/alexandrmotologa/flakeprobe/pkg/suggest"
)

//go:embed web/*
var webFS embed.FS

// Server hosts the embedded interactive cockpit dashboard.
type Server struct {
	Port         int
	report       *report.FullReport
	reportMu     sync.RWMutex
	httpServer   *http.Server
	clients      map[chan string]bool
	clientsMu    sync.Mutex
	orchestrator *runner.Orchestrator
}

// NewServer creates a new cockpit server instance.
func NewServer(port int, initialReport *report.FullReport) *Server {
	orch := runner.NewOrchestrator()
	orch.RegisterAdapter("openai", runner.NewOpenAIAdapter())
	orch.RegisterAdapter("exec", runner.NewExecAdapter())
	orch.RegisterAdapter("mcp", runner.NewMCPAdapter())
	orch.RegisterAdapter("mock", mockagent.NewInProcessMockAdapter())

	return &Server{
		Port:         port,
		report:       initialReport,
		clients:      make(map[chan string]bool),
		orchestrator: orch,
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
	mux.HandleFunc("/api/run", s.handleRunBenchmark)
	mux.HandleFunc("/api/diff", s.handleDiffRuns)
	mux.HandleFunc("/api/suggest", s.handleSuggest)

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

func (s *Server) handleDiffRuns(w http.ResponseWriter, r *http.Request) {
	runAID := r.URL.Query().Get("runA")
	runBID := r.URL.Query().Get("runB")

	s.reportMu.RLock()
	defer s.reportMu.RUnlock()

	if s.report == nil || len(s.report.Runs) == 0 {
		http.Error(w, "No active runs loaded", http.StatusBadRequest)
		return
	}

	var runA, runB *runner.Run
	for i := range s.report.Runs {
		if s.report.Runs[i].ID == runAID {
			runA = &s.report.Runs[i]
		}
		if s.report.Runs[i].ID == runBID {
			runB = &s.report.Runs[i]
		}
	}

	if runA == nil || runB == nil {
		http.Error(w, "One or both run IDs not found", http.StatusNotFound)
		return
	}

	d := diff.CompareRuns(*runA, *runB)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(d)
}

func (s *Server) handleSuggest(w http.ResponseWriter, r *http.Request) {
	s.reportMu.RLock()
	rep := s.report
	s.reportMu.RUnlock()

	if rep == nil || rep.Analysis == nil {
		http.Error(w, "No active analysis report", http.StatusBadRequest)
		return
	}

	mitigations := suggest.GenerateMitigations(rep.Analysis)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(mitigations)
}

func (s *Server) handleRunBenchmark(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		TargetType    string  `json:"target_type"`
		Endpoint      string  `json:"endpoint"`
		Model         string  `json:"model"`
		Prompt        string  `json:"prompt"`
		MockArchetype string  `json:"mock_archetype"`
		Runs          int     `json:"runs"`
		Concurrency   int     `json:"concurrency"`
		MaxFRI        float64 `json:"max_fri"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.TargetType == "" {
		req.TargetType = "mock"
	}
	if req.MockArchetype == "" {
		req.MockArchetype = "bifurcated"
	}
	if req.Runs <= 0 {
		req.Runs = 10
	}
	if req.Concurrency <= 0 {
		req.Concurrency = 2
	}
	if req.MaxFRI <= 0 {
		req.MaxFRI = 0.20
	}

	cfg := runner.SuiteConfig{
		Target: runner.TargetConfig{
			Type:          req.TargetType,
			Endpoint:      req.Endpoint,
			Model:         req.Model,
			Prompt:        req.Prompt,
			MockArchetype: req.MockArchetype,
			MaxSteps:      10,
			TimeoutSec:    60,
		},
		TotalRuns:   req.Runs,
		Concurrency: req.Concurrency,
		MaxFRI:      req.MaxFRI,
	}

	go func() {
		targetDesc := fmt.Sprintf("Web Triggered (%s: %s)", req.TargetType, req.MockArchetype)
		runs, err := s.orchestrator.RunSuite(context.Background(), cfg, func(p runner.StepProgress) {
			evtData, _ := json.Marshal(map[string]interface{}{
				"type":    "step_progress",
				"payload": p,
			})
			s.broadcast(string(evtData))
		})
		if err == nil {
			analysis := entropy.AnalyzeFlakiness(runs, req.MaxFRI, nil)
			fullRep := &report.FullReport{
				Timestamp:  time.Now().UTC(),
				TargetDesc: targetDesc,
				Analysis:   analysis,
				Runs:       runs,
			}
			s.UpdateReport(fullRep)
		}
	}()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "started",
		"message": fmt.Sprintf("Triggered %d runs against %s", req.Runs, req.TargetType),
	})
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
			fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}
