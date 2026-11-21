package runner

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// ExecAdapter executes local agent binaries as subprocesses.
type ExecAdapter struct{}

// NewExecAdapter creates an adapter for running CLI agents.
func NewExecAdapter() *ExecAdapter {
	return &ExecAdapter{}
}

type subprocessEvent struct {
	Type        string                 `json:"type"` // "tool_call", "thought", "completion", "error"
	Tool        string                 `json:"tool,omitempty"`
	ToolName    string                 `json:"tool_name,omitempty"`
	Args        map[string]interface{} `json:"args,omitempty"`
	Arguments   map[string]interface{} `json:"arguments,omitempty"`
	Content     string                 `json:"content,omitempty"`
	Observation string                 `json:"observation,omitempty"`
	Thought     string                 `json:"thought,omitempty"`
	Tokens      int                    `json:"tokens,omitempty"`
}

func (a *ExecAdapter) Execute(ctx context.Context, iteration int, config TargetConfig, onProgress func(StepProgress)) (*Run, error) {
	runID := fmt.Sprintf("run-%03d", iteration)
	startTime := time.Now()

	command := config.ExecCommand
	args := config.ExecArgs

	if command == "" {
		return nil, fmt.Errorf("exec command cannot be empty")
	}

	// Support command line string with spaces if args not provided separately
	if len(args) == 0 && strings.Contains(command, " ") {
		parts := strings.Fields(command)
		command = parts[0]
		args = parts[1:]
	}

	timeoutSec := config.TimeoutSec
	if timeoutSec <= 0 {
		timeoutSec = 120
	}

	execCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(execCtx, command, args...)
	cmd.Env = os.Environ()
	for k, v := range config.Environment {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}
	cmd.Env = append(cmd.Env, fmt.Sprintf("FLAKEPROBE_ITERATION=%d", iteration))
	cmd.Env = append(cmd.Env, fmt.Sprintf("FLAKEPROBE_PROMPT=%s", config.Prompt))

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to open stdout pipe: %w", err)
	}
	defer stdoutPipe.Close()

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start process '%s': %w", command, err)
	}

	var steps []Step
	scanner := bufio.NewScanner(stdoutPipe)
	stepIndex := 0
	totalTokens := 0
	finalOutput := ""

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		now := time.Now()
		var evt subprocessEvent
		if err := json.Unmarshal([]byte(line), &evt); err == nil && (evt.Type != "" || evt.Tool != "" || evt.ToolName != "") {
			tool := evt.Tool
			if tool == "" {
				tool = evt.ToolName
			}
			argsMap := evt.Args
			if argsMap == nil {
				argsMap = evt.Arguments
			}

			actionType := evt.Type
			if actionType == "" && tool != "" {
				actionType = "tool_call"
			}
			if evt.Tokens > 0 {
				totalTokens += evt.Tokens
			}

			isTerm := actionType == "completion"
			if isTerm {
				finalOutput = evt.Content
			}

			st := Step{
				Index:       stepIndex,
				Timestamp:   now,
				DurationMs:  now.Sub(startTime).Milliseconds(),
				ActionType:  actionType,
				ToolName:    tool,
				Arguments:   argsMap,
				Observation: evt.Observation,
				Thought:     evt.Thought,
				IsTerminal:  isTerm,
			}
			steps = append(steps, st)

			if onProgress != nil {
				onProgress(StepProgress{
					RunID:     runID,
					Iteration: iteration,
					StepIndex: stepIndex,
					ToolName:  tool,
					Status:    "step",
				})
			}
			stepIndex++
		} else {
			// Plain text line
			finalOutput = line
		}
	}

	waitErr := cmd.Wait()
	endTime := time.Now()

	// If no structured steps were emitted, synthesize step from process execution
	if len(steps) == 0 {
		steps = append(steps, Step{
			Index:       0,
			Timestamp:   startTime,
			DurationMs:  endTime.Sub(startTime).Milliseconds(),
			ActionType:  "completion",
			Observation: finalOutput,
			IsTerminal:  true,
		})
	}

	runSuccess := (waitErr == nil)
	errMsg := ""
	if waitErr != nil {
		errMsg = waitErr.Error()
	}

	run := &Run{
		ID:           runID,
		Iteration:    iteration,
		StartTime:    startTime,
		EndTime:      endTime,
		DurationMs:   endTime.Sub(startTime).Milliseconds(),
		Steps:        steps,
		TotalTokens:  totalTokens,
		Success:      runSuccess,
		FinalOutput:  finalOutput,
		ErrorMessage: errMsg,
		Metadata: map[string]interface{}{
			"command": command,
			"args":    args,
		},
	}

	return run, nil
}
