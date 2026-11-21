package mockagent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/alexandrmotologa/flakeprobe/pkg/runner"
)

// Server implements an OpenAI-compatible HTTP server providing synthetic agent execution.
type Server struct {
	Archetype Archetype
	Port      int
	server    *http.Server
	counter   int64
}

// NewServer creates a new mock agent server.
func NewServer(port int, archetype Archetype) *Server {
	return &Server{
		Port:      port,
		Archetype: archetype,
	}
}

// Start launches the mock HTTP server.
func (s *Server) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", s.handleChatCompletions)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	s.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", s.Port),
		Handler: mux,
	}

	return s.server.ListenAndServe()
}

// Stop gracefully shuts down the server.
func (s *Server) Stop(ctx context.Context) error {
	if s.server != nil {
		return s.server.Shutdown(ctx)
	}
	return nil
}

func (s *Server) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Model    string `json:"model"`
		Messages []struct {
			Role       string `json:"role"`
			Content    string `json:"content"`
			ToolCallID string `json:"tool_call_id,omitempty"`
		} `json:"messages"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}

	iteration := int(atomic.AddInt64(&s.counter, 1))
	prompt := "mock prompt"
	if len(req.Messages) > 0 {
		prompt = req.Messages[len(req.Messages)-1].Content
	}

	run := GenerateTrajectory(iteration, s.Archetype, prompt)

	// Determine what step in the conversation we are at
	toolCallCount := 0
	for _, m := range req.Messages {
		if m.Role == "tool" {
			toolCallCount++
		}
	}

	w.Header().Set("Content-Type", "application/json")

	if toolCallCount < len(run.Steps) && !run.Steps[toolCallCount].IsTerminal {
		st := run.Steps[toolCallCount]
		argsBytes, _ := json.Marshal(st.Arguments)
		resp := map[string]interface{}{
			"id":     fmt.Sprintf("chatcmpl-mock-%d", iteration),
			"object": "chat.completion",
			"created": time.Now().Unix(),
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": nil,
						"tool_calls": []map[string]interface{}{
							{
								"id":   fmt.Sprintf("call_mock_%d_%d", iteration, toolCallCount),
								"type": "function",
								"function": map[string]interface{}{
									"name":      st.ToolName,
									"arguments": string(argsBytes),
								},
							},
						},
					},
					"finish_reason": "tool_calls",
				},
			},
			"usage": map[string]int{
				"prompt_tokens":     120,
				"completion_tokens": 40,
				"total_tokens":      160,
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	} else {
		// Terminal response
		resp := map[string]interface{}{
			"id":     fmt.Sprintf("chatcmpl-mock-%d", iteration),
			"object": "chat.completion",
			"created": time.Now().Unix(),
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": run.FinalOutput,
					},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]int{
				"prompt_tokens":     100,
				"completion_tokens": 30,
				"total_tokens":      130,
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}
}

// InProcessMockAdapter allows running mock archetypes directly in memory without starting an HTTP server.
type InProcessMockAdapter struct{}

// NewInProcessMockAdapter returns an in-process mock runner.
func NewInProcessMockAdapter() *InProcessMockAdapter {
	return &InProcessMockAdapter{}
}

func (m *InProcessMockAdapter) Execute(ctx context.Context, iteration int, config runner.TargetConfig, onProgress func(runner.StepProgress)) (*runner.Run, error) {
	arch := Archetype(config.MockArchetype)
	if arch == "" {
		arch = ArchetypeDeterministic
	}

	run := GenerateTrajectory(iteration, arch, config.Prompt)

	if onProgress != nil {
		for _, st := range run.Steps {
			onProgress(runner.StepProgress{
				RunID:     run.ID,
				Iteration: iteration,
				StepIndex: st.Index,
				ToolName:  st.ToolName,
				Status:    "step",
			})
		}
	}

	return run, nil
}
