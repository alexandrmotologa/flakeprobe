package test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/alexandrmotologa/flakeprobe/pkg/recorder"
)

func TestReplayServerHTTP(t *testing.T) {
	fixture := &recorder.ReplayFixture{
		Version:     "flakeprobe/v1",
		TotalSteps:  2,
		FinalOutput: "Task completed deterministically.",
		Steps: []recorder.FixtureStep{
			{
				Index:      0,
				ActionType: "tool_call",
				ToolName:   "verify_account",
				Arguments: map[string]interface{}{
					"account_id": "acc_12345",
				},
				Thought:     "Checking account status",
				Observation: `{"valid":true}`,
			},
			{
				Index:       1,
				ActionType:  "completion",
				Observation: "Task completed deterministically.",
				IsTerminal:  true,
			},
		},
	}

	server := recorder.NewReplayServer(fixture, 18492, true)
	go func() {
		_ = server.Start()
	}()
	time.Sleep(50 * time.Millisecond) // Allow server to bind
	defer func() {
		_ = server.Stop(context.Background())
	}()

	// 1. Send first turn request
	reqBody := map[string]interface{}{
		"model": "gpt-4o",
		"messages": []map[string]interface{}{
			{"role": "user", "content": "Verify account acc_12345"},
		},
	}
	bodyBytes, _ := json.Marshal(reqBody)

	resp, err := http.Post("http://127.0.0.1:18492/v1/chat/completions", "application/json", bytes.NewReader(bodyBytes))
	if err != nil {
		t.Fatalf("POST to replay server failed: %v", err)
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(resp.Body)
	var chatResp struct {
		Choices []struct {
			Message struct {
				Role      string `json:"role"`
				ToolCalls []struct {
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBytes, &chatResp); err != nil {
		t.Fatalf("failed to decode replay response: %v", err)
	}

	if len(chatResp.Choices) == 0 || len(chatResp.Choices[0].Message.ToolCalls) == 0 {
		t.Fatalf("expected tool call choice in response, got: %s", string(respBytes))
	}

	toolCall := chatResp.Choices[0].Message.ToolCalls[0]
	if toolCall.Function.Name != "verify_account" {
		t.Errorf("expected tool verify_account, got %s", toolCall.Function.Name)
	}

	// 2. Send second turn with tool observation
	reqBodyTurn2 := map[string]interface{}{
		"model": "gpt-4o",
		"messages": []map[string]interface{}{
			{"role": "user", "content": "Verify account acc_12345"},
			{"role": "assistant", "content": "Checking account status"},
			{"role": "tool", "content": `{"valid":true}`},
		},
	}
	bodyBytes2, _ := json.Marshal(reqBodyTurn2)

	resp2, err := http.Post("http://127.0.0.1:18492/v1/chat/completions", "application/json", bytes.NewReader(bodyBytes2))
	if err != nil {
		t.Fatalf("POST turn 2 failed: %v", err)
	}
	defer resp2.Body.Close()

	respBytes2, _ := io.ReadAll(resp2.Body)
	var termResp struct {
		Choices []struct {
			Message struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBytes2, &termResp); err != nil {
		t.Fatalf("failed to decode turn 2 response: %v", err)
	}

	if len(termResp.Choices) == 0 || termResp.Choices[0].Message.Content != "Task completed deterministically." {
		t.Errorf("expected terminal content, got: %s", string(respBytes2))
	}
}
