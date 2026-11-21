package runner

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// MCPAdapter executes agent interactions against Model Context Protocol servers over stdio.
type MCPAdapter struct{}

// NewMCPAdapter creates a driver for MCP stdio servers.
func NewMCPAdapter() *MCPAdapter {
	return &MCPAdapter{}
}

type jsonRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int64       `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
}

type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type mcpToolsListResult struct {
	Tools []struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		InputSchema json.RawMessage `json:"inputSchema"`
	} `json:"tools"`
}

type mcpToolCallParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

type mcpToolCallResult struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	IsError bool `json:"isError"`
}

func (m *MCPAdapter) Execute(ctx context.Context, iteration int, config TargetConfig, onProgress func(StepProgress)) (*Run, error) {
	runID := fmt.Sprintf("run-%03d", iteration)
	startTime := time.Now()

	command := config.MCPCommand
	args := config.MCPArgs
	if command == "" {
		command = config.ExecCommand
		args = config.ExecArgs
	}
	if command == "" {
		return nil, fmt.Errorf("mcp command cannot be empty")
	}

	cmd := exec.CommandContext(ctx, command, args...)
	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("mcp stdin pipe error: %w", err)
	}
	defer stdinPipe.Close()

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("mcp stdout pipe error: %w", err)
	}
	defer stdoutPipe.Close()

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start mcp process '%s': %w", command, err)
	}
	defer func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}()

	var reqCounter int64
	pendingMu := sync.Mutex{}
	pending := make(map[int64]chan jsonRPCResponse)

	scanner := bufio.NewScanner(stdoutPipe)
	go func() {
		for scanner.Scan() {
			line := scanner.Bytes()
			var resp jsonRPCResponse
			if err := json.Unmarshal(line, &resp); err == nil && resp.ID > 0 {
				pendingMu.Lock()
				ch, exists := pending[resp.ID]
				if exists {
					delete(pending, resp.ID)
					ch <- resp
				}
				pendingMu.Unlock()
			}
		}
	}()

	sendRequest := func(method string, params interface{}) (*jsonRPCResponse, error) {
		reqID := atomic.AddInt64(&reqCounter, 1)
		req := jsonRPCRequest{
			JSONRPC: "2.0",
			ID:      reqID,
			Method:  method,
			Params:  params,
		}

		data, err := json.Marshal(req)
		if err != nil {
			return nil, err
		}

		respCh := make(chan jsonRPCResponse, 1)
		pendingMu.Lock()
		pending[reqID] = respCh
		pendingMu.Unlock()

		if _, err := io.WriteString(stdinPipe, string(data)+"\n"); err != nil {
			pendingMu.Lock()
			delete(pending, reqID)
			pendingMu.Unlock()
			return nil, err
		}

		select {
		case resp := <-respCh:
			if resp.Error != nil {
				return nil, fmt.Errorf("mcp rpc error %d: %s", resp.Error.Code, resp.Error.Message)
			}
			return &resp, nil
		case <-time.After(10 * time.Second):
			pendingMu.Lock()
			delete(pending, reqID)
			pendingMu.Unlock()
			return nil, fmt.Errorf("mcp request timeout for method %s", method)
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	// 1. Initialize
	initParams := map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities": map[string]interface{}{
			"tools": map[string]interface{}{},
		},
		"clientInfo": map[string]string{
			"name":    "flakeprobe",
			"version": "1.0.0",
		},
	}
	if _, err := sendRequest("initialize", initParams); err != nil {
		return nil, fmt.Errorf("mcp handshake failed: %w", err)
	}

	// 2. Notification initialized
	initNotif := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "notifications/initialized",
	}
	notifData, _ := json.Marshal(initNotif)
	_, _ = io.WriteString(stdinPipe, string(notifData)+"\n")

	// 3. Discover tools
	toolsResp, err := sendRequest("tools/list", map[string]interface{}{})
	if err != nil {
		return nil, fmt.Errorf("mcp tools/list failed: %w", err)
	}

	var toolsList mcpToolsListResult
	_ = json.Unmarshal(toolsResp.Result, &toolsList)

	var steps []Step
	stepIndex := 0

	// Execute tool calls defined by prompt or available tools
	for _, tool := range toolsList.Tools {
		stepStart := time.Now()
		callParams := mcpToolCallParams{
			Name: tool.Name,
			Arguments: map[string]interface{}{
				"prompt":    config.Prompt,
				"iteration": iteration,
			},
		}

		callResp, err := sendRequest("tools/call", callParams)
		durMs := time.Since(stepStart).Milliseconds()

		obsText := ""
		isErr := false
		if err != nil {
			obsText = err.Error()
			isErr = true
		} else {
			var res mcpToolCallResult
			_ = json.Unmarshal(callResp.Result, &res)
			isErr = res.IsError
			var texts []string
			for _, c := range res.Content {
				texts = append(texts, c.Text)
			}
			obsText = strings.Join(texts, "\n")
		}

		st := Step{
			Index:       stepIndex,
			Timestamp:   stepStart,
			DurationMs:  durMs,
			ActionType:  "tool_call",
			ToolName:    tool.Name,
			Arguments:   callParams.Arguments,
			Observation: obsText,
			IsTerminal:  false,
		}
		if isErr {
			st.Error = obsText
		}
		steps = append(steps, st)

		if onProgress != nil {
			onProgress(StepProgress{
				RunID:     runID,
				Iteration: iteration,
				StepIndex: stepIndex,
				ToolName:  tool.Name,
				Status:    "step",
			})
		}
		stepIndex++
	}

	endTime := time.Now()
	steps = append(steps, Step{
		Index:       stepIndex,
		Timestamp:   endTime,
		DurationMs:  1,
		ActionType:  "completion",
		Observation: "MCP session completed successfully",
		IsTerminal:  true,
	})

	run := &Run{
		ID:          runID,
		Iteration:   iteration,
		StartTime:   startTime,
		EndTime:     endTime,
		DurationMs:  endTime.Sub(startTime).Milliseconds(),
		Steps:       steps,
		Success:     true,
		FinalOutput: fmt.Sprintf("Completed %d MCP tool invocations", len(toolsList.Tools)),
		Metadata: map[string]interface{}{
			"tools_count": len(toolsList.Tools),
		},
	}

	return run, nil
}
