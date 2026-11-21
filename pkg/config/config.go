package config

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/alexandrmotologa/flakeprobe/pkg/runner"
)

// Config represents a declarative flakeprobe test configuration.
type Config struct {
	Version string           `json:"version"`
	Target  TargetSettings   `json:"target"`
	Suite   SuiteSettings    `json:"suite"`
	Sweep   SweepSettings    `json:"sweep,omitempty"`
	Reports ReportSettings   `json:"reports,omitempty"`
}

type TargetSettings struct {
	Type          string            `json:"type"` // "openai", "exec", "mcp", "mock"
	Endpoint      string            `json:"endpoint,omitempty"`
	APIKey        string            `json:"api_key,omitempty"`
	Model         string            `json:"model,omitempty"`
	Prompt        string            `json:"prompt"`
	SystemPrompt  string            `json:"system_prompt,omitempty"`
	ExecCommand   string            `json:"exec_command,omitempty"`
	MCPCommand    string            `json:"mcp_command,omitempty"`
	MockArchetype string            `json:"mock_archetype,omitempty"`
	Environment   map[string]string `json:"environment,omitempty"`
}

type SuiteSettings struct {
	Runs        int     `json:"runs"`
	Concurrency int     `json:"concurrency"`
	JitterMs    int     `json:"jitter_ms"`
	MaxFRI      float64 `json:"max_fri"`
}

type SweepSettings struct {
	Temperatures []float64 `json:"temperatures,omitempty"`
	Prompts      []string  `json:"prompts,omitempty"`
}

type ReportSettings struct {
	Out           string `json:"out,omitempty"`
	JUnit         string `json:"junit,omitempty"`
	GitHubSummary string `json:"github_summary,omitempty"`
}

// DefaultConfig returns a ready-to-use configuration.
func DefaultConfig() *Config {
	return &Config{
		Version: "1",
		Target: TargetSettings{
			Type:          "mock",
			MockArchetype: "deterministic",
			Prompt:        "Execute system reliability check",
		},
		Suite: SuiteSettings{
			Runs:        10,
			Concurrency: 4,
			MaxFRI:      0.20,
		},
	}
}

// LoadConfig parses a config file (.json or .yaml).
func LoadConfig(filePath string) (*Config, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	ext := strings.ToLower(filepath.Ext(filePath))
	if ext == ".json" {
		var cfg Config
		if err := json.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("failed to parse json config: %w", err)
		}
		applyDefaults(&cfg)
		return &cfg, nil
	}

	// Simple YAML-like line parser for zero external dependency
	cfg, err := parseSimpleYAML(string(data))
	if err != nil {
		// Fallback to JSON unmarshal in case it's JSON formatted inside .yaml
		var jsonCfg Config
		if jsonErr := json.Unmarshal(data, &jsonCfg); jsonErr == nil {
			applyDefaults(&jsonCfg)
			return &jsonCfg, nil
		}
		return nil, err
	}

	applyDefaults(cfg)
	return cfg, nil
}

func applyDefaults(cfg *Config) {
	if cfg.Suite.Runs <= 0 {
		cfg.Suite.Runs = 10
	}
	if cfg.Suite.Concurrency <= 0 {
		cfg.Suite.Concurrency = 2
	}
	if cfg.Suite.MaxFRI <= 0 {
		cfg.Suite.MaxFRI = 0.20
	}
}

func parseSimpleYAML(text string) (*Config, error) {
	cfg := DefaultConfig()
	lines := strings.Split(text, "\n")
	currentSection := ""

	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasSuffix(line, ":") {
			currentSection = strings.TrimSuffix(line, ":")
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		val = strings.Trim(val, `"'`)

		switch currentSection {
		case "target":
			switch key {
			case "type":
				cfg.Target.Type = val
			case "endpoint":
				cfg.Target.Endpoint = val
			case "model":
				cfg.Target.Model = val
			case "prompt":
				cfg.Target.Prompt = val
			case "mock_archetype":
				cfg.Target.MockArchetype = val
			case "exec_command":
				cfg.Target.ExecCommand = val
			case "mcp_command":
				cfg.Target.MCPCommand = val
			}
		case "suite":
			switch key {
			case "runs":
				if n, err := strconv.Atoi(val); err == nil {
					cfg.Suite.Runs = n
				}
			case "concurrency":
				if n, err := strconv.Atoi(val); err == nil {
					cfg.Suite.Concurrency = n
				}
			case "max_fri":
				if f, err := strconv.ParseFloat(val, 64); err == nil {
					cfg.Suite.MaxFRI = f
				}
			case "jitter_ms":
				if n, err := strconv.Atoi(val); err == nil {
					cfg.Suite.JitterMs = n
				}
			}
		case "reports":
			switch key {
			case "out":
				cfg.Reports.Out = val
			case "junit":
				cfg.Reports.JUnit = val
			}
		case "sweep":
			if key == "temperatures" {
				val = strings.Trim(val, "[]")
				for _, p := range strings.Split(val, ",") {
					if f, err := strconv.ParseFloat(strings.TrimSpace(p), 64); err == nil {
						cfg.Sweep.Temperatures = append(cfg.Sweep.Temperatures, f)
					}
				}
			}
		}
	}

	return cfg, nil
}

// SweepEntry records flakiness profile for a specific parameter permutation.
type SweepEntry struct {
	Temperature float64 `json:"temperature"`
	Prompt      string  `json:"prompt"`
	FRI         float64 `json:"fri"`
	MaxEntropy  float64 `json:"max_entropy"`
	Archetype   string  `json:"archetype"`
	Passed      bool    `json:"passed"`
}

// SweepResult aggregates results of a temperature/prompt sweep.
type SweepResult struct {
	Entries               []SweepEntry `json:"entries"`
	PhaseTransitionTemp   float64      `json:"phase_transition_temp"` // Temperature where agent begins failing FRI gate (-1 if none)
	OptimalTemperature    float64      `json:"optimal_temperature"`
}

// RenderTerminalSweep prints the sweep results table to stdout.
func RenderTerminalSweep(w io.Writer, res *SweepResult) {
	if w == nil {
		w = os.Stdout
	}

	reset := "\033[0m"
	bold := "\033[1m"
	cyan := "\033[36m"
	green := "\033[32m"
	red := "\033[31m"

	fmt.Fprintf(w, "\n%s%s=== flakeprobe: Parameter Sweep & Phase Transition Matrix ===%s\n", bold, cyan, reset)
	if res.PhaseTransitionTemp >= 0 {
		fmt.Fprintf(w, "Phase Transition Point: %sT = %.2f%s (Agent transitions from stable to flaky)\n", red, res.PhaseTransitionTemp, reset)
	} else {
		fmt.Fprintf(w, "Phase Transition Point: %sNone (Agent remains stable across all tested temperatures)%s\n", green, reset)
	}
	fmt.Fprintf(w, "Optimal Recommended Temp: %sT = %.2f%s\n", green, res.OptimalTemperature, reset)
	fmt.Fprintln(w, strings.Repeat("-", 76))
	fmt.Fprintf(w, "%-12s %-12s %-16s %-16s %-12s\n", "TEMP (T)", "FRI", "MAX H(s)", "ARCHETYPE", "STATUS")
	fmt.Fprintln(w, strings.Repeat("-", 76))

	for _, e := range res.Entries {
		statusStr := fmt.Sprintf("%sPASS%s", green, reset)
		if !e.Passed {
			statusStr = fmt.Sprintf("%sFAIL%s", red, reset)
		}
		fmt.Fprintf(w, "T = %-8.2f %-12.4f %-16.4f %-16s %-12s\n", e.Temperature, e.FRI, e.MaxEntropy, e.Archetype, statusStr)
	}
	fmt.Fprintln(w, "")
}

// ToRunnerConfig converts TargetSettings to runner.TargetConfig.
func (t *TargetSettings) ToRunnerConfig() runner.TargetConfig {
	return runner.TargetConfig{
		Type:          t.Type,
		Endpoint:      t.Endpoint,
		APIKey:        t.APIKey,
		Model:         t.Model,
		Prompt:        t.Prompt,
		SystemPrompt:  t.SystemPrompt,
		ExecCommand:   t.ExecCommand,
		MCPCommand:    t.MCPCommand,
		MockArchetype: t.MockArchetype,
		MaxSteps:      10,
		TimeoutSec:    60,
	}
}
