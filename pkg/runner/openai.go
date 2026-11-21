package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// OpenAIAdapter executes agents targeting OpenAI-compatible endpoints with tool calling.
type OpenAIAdapter struct {
	Client *http.Client
}

// NewOpenAIAdapter creates a new adapter with default HTTP client.
func NewOpenAIAdapter() *OpenAIAdapter {
	return &OpenAIAdapter{
		Client: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

type openAIMessage struct {
	Role       string           `json:"role"`
	Content    interface{}      `json:"content"`
	ToolCalls  []openAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
}

type openAIToolCall struct {
	ID       string             `json:"id"`
	Type     string             `json:"type"`
	Function openAIFuncCallData `json:"function"`
}

type openAIFuncCallData struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type openAIChatRequest struct {
	Model       string          `json:"model"`
	Messages    []openAIMessage `json:"messages"`
	Temperature float64         `json:"temperature"`
	Tools       []interface{}   `json:"tools,omitempty"`
}

type openAIChatResponse struct {
	ID      string `json:"id"`
	Choices []struct {
		Index        int           `json:"index"`
		Message      openAIMessage `json:"message"`
		FinishReason string        `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error,omitempty"`
}

func (a *OpenAIAdapter) Execute(ctx context.Context, iteration int, config TargetConfig, onProgress func(StepProgress)) (*Run, error) {
	runID := fmt.Sprintf("run-%03d", iteration)
	startTime := time.Now()

	maxSteps := config.MaxSteps
	if maxSteps <= 0 {
		maxSteps = 10
	}

	endpoint := strings.TrimRight(config.Endpoint, "/")
	if !strings.HasSuffix(endpoint, "/chat/completions") {
		endpoint = endpoint + "/chat/completions"
	}

	messages := make([]openAIMessage, 0)
	if config.SystemPrompt != "" {
		messages = append(messages, openAIMessage{
			Role:    "system",
			Content: config.SystemPrompt,
		})
	}
	messages = append(messages, openAIMessage{
		Role:    "user",
		Content: config.Prompt,
	})

	var steps []Step
	totalTokens := 0
	stepIndex := 0

	run := &Run{
		ID:        runID,
		Iteration: iteration,
		StartTime: startTime,
		Steps:     steps,
		Success:   true,
		Metadata:  make(map[string]interface{}),
	}

	for stepIndex < maxSteps {
		stepStart := time.Now()
		reqBody := openAIChatRequest{
			Model:       config.Model,
			Messages:    messages,
			Temperature: config.Temperature,
		}

		bodyBytes, err := json.Marshal(reqBody)
		if err != nil {
			return nil, fmt.Errorf("marshal request failed: %w", err)
		}

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
		if err != nil {
			return nil, fmt.Errorf("create http request failed: %w", err)
		}

		httpReq.Header.Set("Content-Type", "application/json")
		if config.APIKey != "" {
			httpReq.Header.Set("Authorization", "Bearer "+config.APIKey)
		}

		resp, err := a.Client.Do(httpReq)
		if err != nil {
			return nil, fmt.Errorf("http request error: %w", err)
		}

		respBytes, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("read response error: %w", err)
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, fmt.Errorf("openai endpoint returned HTTP %d: %s", resp.StatusCode, string(respBytes))
		}

		var chatResp openAIChatResponse
		if err := json.Unmarshal(respBytes, &chatResp); err != nil {
			return nil, fmt.Errorf("unmarshal chat response error: %w", err)
		}

		if chatResp.Error != nil {
			return nil, fmt.Errorf("openai api error: %s", chatResp.Error.Message)
		}

		if len(chatResp.Choices) == 0 {
			return nil, fmt.Errorf("openai response contained zero choices")
		}

		choice := chatResp.Choices[0]
		totalTokens += chatResp.Usage.TotalTokens
		durationMs := time.Since(stepStart).Milliseconds()

		if len(choice.Message.ToolCalls) > 0 {
			for _, tc := range choice.Message.ToolCalls {
				var parsedArgs map[string]interface{}
				_ = json.Unmarshal([]byte(tc.Function.Arguments), &parsedArgs)

				st := Step{
					Index:       stepIndex,
					Timestamp:   stepStart,
					DurationMs:  durationMs,
					ActionType:  "tool_call",
					ToolName:    tc.Function.Name,
					Arguments:   parsedArgs,
					Thought:     fmt.Sprintf("%v", choice.Message.Content),
					IsTerminal:  false,
					Observation: fmt.Sprintf(`{"status":"success","result":"ok_%s"}`, tc.Function.Name),
				}
				steps = append(steps, st)

				if onProgress != nil {
					onProgress(StepProgress{
						RunID:     runID,
						Iteration: iteration,
						StepIndex: stepIndex,
						ToolName:  tc.Function.Name,
						Status:    "step",
					})
				}

				messages = append(messages, choice.Message)
				messages = append(messages, openAIMessage{
					Role:       "tool",
					Content:    st.Observation,
					ToolCallID: tc.ID,
				})
				stepIndex++
			}
		} else {
			// Completion step
			contentStr := ""
			if choice.Message.Content != nil {
				contentStr = fmt.Sprintf("%v", choice.Message.Content)
			}

			st := Step{
				Index:       stepIndex,
				Timestamp:   stepStart,
				DurationMs:  durationMs,
				ActionType:  "completion",
				Thought:     contentStr,
				IsTerminal:  true,
				Observation: contentStr,
			}
			steps = append(steps, st)
			run.FinalOutput = contentStr
			break
		}
	}

	run.EndTime = time.Now()
	run.DurationMs = run.EndTime.Sub(startTime).Milliseconds()
	run.Steps = steps
	run.TotalTokens = totalTokens

	return run, nil
}
