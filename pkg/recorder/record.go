package recorder

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/alexandrmotologa/flakeprobe/pkg/entropy"
	"github.com/alexandrmotologa/flakeprobe/pkg/runner"
)

// FixtureStep records an expected tool call and its deterministic mock observation.
type FixtureStep struct {
	Index       int                    `json:"index"`
	ActionType  string                 `json:"action_type"`
	ToolName    string                 `json:"tool_name,omitempty"`
	Arguments   map[string]interface{} `json:"arguments,omitempty"`
	Observation string                 `json:"observation,omitempty"`
	Thought     string                 `json:"thought,omitempty"`
	IsTerminal  bool                   `json:"is_terminal"`
}

// ReplayFixture encapsulates a deterministic test contract for CI/CD replay.
type ReplayFixture struct {
	Version        string                 `json:"version"`
	GeneratedAt    time.Time              `json:"generated_at"`
	Checksum       string                 `json:"checksum"`
	Prompt         string                 `json:"prompt"`
	TargetType     string                 `json:"target_type"`
	TotalSteps     int                    `json:"total_steps"`
	GoldenRunID    string                 `json:"golden_run_id"`
	Steps          []FixtureStep          `json:"steps"`
	FinalOutput    string                 `json:"final_output"`
	PathConfidence float64                `json:"path_confidence"` // Empirical probability of golden path
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
}

// SynthesizeFixture extracts the golden path and produces a deterministic replay contract.
func SynthesizeFixture(runs []runner.Run, analysis *entropy.FlakinessAnalysis, prompt string, targetType string) (*ReplayFixture, error) {
	if len(runs) == 0 {
		return nil, fmt.Errorf("cannot synthesize fixture from empty runs")
	}

	// Find the golden run: highest probability successful trajectory
	goldenRunIdx := 0
	maxPathProb := -1.0

	for i, r := range runs {
		if !r.Success {
			continue
		}
		// Calculate trajectory joint probability along DAG edges
		prob := 1.0
		prevNode := "START"
		for idx, st := range r.Steps {
			nodeID := fmt.Sprintf("step_%d:%s", idx, st.ToolName)
			if st.IsTerminal {
				nodeID = fmt.Sprintf("step_%d:TERMINAL", idx)
			}

			// Find edge prob
			edgeProb := 1.0
			if analysis != nil && analysis.StateDAG != nil {
				for _, edge := range analysis.StateDAG.Edges {
					if edge.Source == prevNode && edge.Target == nodeID {
						edgeProb = edge.Probability
						break
					}
				}
			}
			prob *= edgeProb
			prevNode = nodeID
		}

		if prob > maxPathProb {
			maxPathProb = prob
			goldenRunIdx = i
		}
	}

	golden := runs[goldenRunIdx]
	var fixtureSteps []FixtureStep

	for _, s := range golden.Steps {
		fixtureSteps = append(fixtureSteps, FixtureStep{
			Index:       s.Index,
			ActionType:  s.ActionType,
			ToolName:    s.ToolName,
			Arguments:   s.Arguments,
			Observation: s.Observation,
			Thought:     s.Thought,
			IsTerminal:  s.IsTerminal,
		})
	}

	fixture := &ReplayFixture{
		Version:        "flakeprobe/v1",
		GeneratedAt:    time.Now().UTC(),
		Prompt:         prompt,
		TargetType:     targetType,
		TotalSteps:     len(fixtureSteps),
		GoldenRunID:    golden.ID,
		Steps:          fixtureSteps,
		FinalOutput:    golden.FinalOutput,
		PathConfidence: maxPathProb,
		Metadata: map[string]interface{}{
			"sample_runs": len(runs),
			"fri":         analysis.FRI,
			"archetype":   analysis.Archetype,
		},
	}

	// Compute checksum
	data, _ := json.Marshal(fixtureSteps)
	hash := sha256.Sum256(data)
	fixture.Checksum = hex.EncodeToString(hash[:])

	return fixture, nil
}

// SaveFixture writes the fixture to disk with standard .flakerec.json format.
func SaveFixture(fixture *ReplayFixture, filePath string) error {
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	data, err := json.MarshalIndent(fixture, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal fixture: %w", err)
	}

	return os.WriteFile(filePath, data, 0644)
}

// LoadFixture reads and parses a fixture file from disk.
func LoadFixture(filePath string) (*ReplayFixture, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read fixture file: %w", err)
	}

	var fixture ReplayFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		return nil, fmt.Errorf("failed to parse fixture json: %w", err)
	}

	return &fixture, nil
}
