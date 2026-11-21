package recorder

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"sync"
	"time"

	"github.com/alexandrmotologa/flakeprobe/pkg/runner"
)

// ReplayServer serves recorded fixtures deterministically for fast CI/CD integration.
type ReplayServer struct {
	Fixture   *ReplayFixture
	Strict    bool
	Port      int
	server    *http.Server
	stepMu    sync.Mutex
	stepIndex int
	Mismatches []string
}

// NewReplayServer creates a deterministic mock replay server.
func NewReplayServer(fixture *ReplayFixture, port int, strict bool) *ReplayServer {
	return &ReplayServer{
		Fixture:   fixture,
		Port:      port,
		Strict:    strict,
		Mismatches: make([]string, 0),
	}
}

// Start initiates the replay mock HTTP server.
func (rs *ReplayServer) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", rs.handleChatCompletions)
	mux.HandleFunc("/replay/status", rs.handleStatus)

	rs.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", rs.Port),
		Handler: mux,
	}

	return rs.server.ListenAndServe()
}

// Stop terminates the replay server.
func (rs *ReplayServer) Stop(ctx context.Context) error {
	if rs.server != nil {
		return rs.server.Shutdown(ctx)
	}
	return nil
}

func (rs *ReplayServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	rs.stepMu.Lock()
	defer rs.stepMu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"current_step": rs.stepIndex,
		"total_steps":  len(rs.Fixture.Steps),
		"mismatches":   rs.Mismatches,
		"passed":       len(rs.Mismatches) == 0,
	})
}

func (rs *ReplayServer) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Model    string `json:"model"`
		Messages []struct {
			Role       string      `json:"role"`
			Content    interface{} `json:"content"`
			ToolCallID string      `json:"tool_call_id,omitempty"`
		} `json:"messages"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}

	rs.stepMu.Lock()
	defer rs.stepMu.Unlock()

	// Check conversation history depth
	toolMsgCount := 0
	for _, m := range req.Messages {
		if m.Role == "tool" {
			toolMsgCount++
		}
	}

	currIdx := toolMsgCount
	if currIdx >= len(rs.Fixture.Steps) {
		// Terminal response
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":      "replay-complete",
			"object":  "chat.completion",
			"created": time.Now().Unix(),
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": rs.Fixture.FinalOutput,
					},
					"finish_reason": "stop",
				},
			},
		})
		return
	}

	expectedStep := rs.Fixture.Steps[currIdx]

	if expectedStep.IsTerminal {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":      fmt.Sprintf("replay-step-%d", currIdx),
			"object":  "chat.completion",
			"created": time.Now().Unix(),
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]interface{}{
						"role":    "assistant",
						"content": expectedStep.Observation,
					},
					"finish_reason": "stop",
				},
			},
		})
		return
	}

	// Tool call response
	argsBytes, _ := json.Marshal(expectedStep.Arguments)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"id":      fmt.Sprintf("replay-step-%d", currIdx),
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"choices": []map[string]interface{}{
			{
				"index": 0,
				"message": map[string]interface{}{
					"role":    "assistant",
					"content": expectedStep.Thought,
					"tool_calls": []map[string]interface{}{
						{
							"id":   fmt.Sprintf("call_replay_%d", currIdx),
							"type": "function",
							"function": map[string]interface{}{
								"name":      expectedStep.ToolName,
								"arguments": string(argsBytes),
							},
						},
					},
				},
				"finish_reason": "tool_calls",
			},
		},
	})
	rs.stepIndex++
}

// ReplayResult encapsulates the outcome of verifying an actual run against a fixture.
type ReplayResult struct {
	Matched     bool     `json:"matched"`
	Differences []string `json:"differences"`
	StepsTested int      `json:"steps_tested"`
}

// VerifyRunAgainstFixture checks if an executed run conforms strictly to a recorded golden contract.
func VerifyRunAgainstFixture(run runner.Run, fixture ReplayFixture, strict bool) ReplayResult {
	var diffs []string

	if len(run.Steps) != len(fixture.Steps) {
		diffs = append(diffs, fmt.Sprintf("step count mismatch: got %d steps, fixture expects %d steps", len(run.Steps), len(fixture.Steps)))
	}

	limit := len(run.Steps)
	if len(fixture.Steps) < limit {
		limit = len(fixture.Steps)
	}

	for i := 0; i < limit; i++ {
		actual := run.Steps[i]
		expected := fixture.Steps[i]

		if actual.ActionType != expected.ActionType {
			diffs = append(diffs, fmt.Sprintf("step %d action type mismatch: got %s, expected %s", i, actual.ActionType, expected.ActionType))
		}

		if actual.ToolName != expected.ToolName {
			diffs = append(diffs, fmt.Sprintf("step %d tool name mismatch: got %s, expected %s", i, actual.ToolName, expected.ToolName))
		}

		if strict && actual.ActionType == "tool_call" {
			if !reflect.DeepEqual(actual.Arguments, expected.Arguments) {
				diffs = append(diffs, fmt.Sprintf("step %d arguments mismatch for tool %s: got %v, expected %v", i, actual.ToolName, actual.Arguments, expected.Arguments))
			}
		}
	}

	return ReplayResult{
		Matched:     len(diffs) == 0,
		Differences: diffs,
		StepsTested: limit,
	}
}
