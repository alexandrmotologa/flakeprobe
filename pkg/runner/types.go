package runner

import (
	"time"
)

// Step represents a single discrete transition or action taken by an agent.
type Step struct {
	Index        int                    `json:"index"`
	Timestamp    time.Time              `json:"timestamp"`
	DurationMs   int64                  `json:"duration_ms"`
	ActionType   string                 `json:"action_type"` // e.g. "tool_call", "thought", "completion", "error"
	ToolName     string                 `json:"tool_name,omitempty"`
	Arguments    map[string]interface{} `json:"arguments,omitempty"`
	Observation  string                 `json:"observation,omitempty"`
	Thought      string                 `json:"thought,omitempty"`
	IsTerminal   bool                   `json:"is_terminal"`
	Error        string                 `json:"error,omitempty"`
}

// Run encapsulates an entire single execution trajectory of an agent.
type Run struct {
	ID           string                 `json:"id"`
	Iteration    int                    `json:"iteration"`
	StartTime    time.Time              `json:"start_time"`
	EndTime      time.Time              `json:"end_time"`
	DurationMs   int64                  `json:"duration_ms"`
	Steps        []Step                 `json:"steps"`
	TotalTokens  int                    `json:"total_tokens"`
	Success      bool                   `json:"success"`
	FinalOutput  string                 `json:"final_output"`
	ErrorMessage string                 `json:"error_message,omitempty"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// TargetConfig holds parameters for executing an agent target.
type TargetConfig struct {
	Type          string            `json:"type"` // "openai", "exec", "mcp", "mock"
	Endpoint      string            `json:"endpoint,omitempty"`
	APIKey        string            `json:"api_key,omitempty"`
	Model         string            `json:"model,omitempty"`
	Prompt        string            `json:"prompt"`
	SystemPrompt  string            `json:"system_prompt,omitempty"`
	ExecCommand   string            `json:"exec_command,omitempty"`
	ExecArgs      []string          `json:"exec_args,omitempty"`
	MCPCommand    string            `json:"mcp_command,omitempty"`
	MCPArgs       []string          `json:"mcp_args,omitempty"`
	MockArchetype string            `json:"mock_archetype,omitempty"`
	MaxSteps      int               `json:"max_steps"`
	TimeoutSec    int               `json:"timeout_sec"`
	Temperature   float64           `json:"temperature"`
	Environment   map[string]string `json:"environment,omitempty"`
}

// SuiteConfig configures a multi-run flakiness profiling campaign.
type SuiteConfig struct {
	Target      TargetConfig `json:"target"`
	TotalRuns   int          `json:"total_runs"`
	Concurrency int          `json:"concurrency"`
	JitterMs    int          `json:"jitter_ms"`
	MaxFRI      float64      `json:"max_fri"`
}

// StepProgress reports real-time execution events.
type StepProgress struct {
	RunID     string `json:"run_id"`
	Iteration int    `json:"iteration"`
	StepIndex int    `json:"step_index"`
	ToolName  string `json:"tool_name"`
	Status    string `json:"status"` // "started", "step", "completed", "failed"
}
