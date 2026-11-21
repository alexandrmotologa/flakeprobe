package test

import (
	"math"
	"testing"
	"time"

	"github.com/alexandrmotologa/flakeprobe/pkg/entropy"
	"github.com/alexandrmotologa/flakeprobe/pkg/mockagent"
	"github.com/alexandrmotologa/flakeprobe/pkg/runner"
)

func TestDeterministicEntropy(t *testing.T) {
	runs := make([]runner.Run, 10)
	for i := 0; i < 10; i++ {
		runs[i] = *mockagent.GenerateTrajectory(i+1, mockagent.ArchetypeDeterministic, "test")
	}

	analysis := entropy.AnalyzeFlakiness(runs, 0.20, nil)

	if analysis.FRI != 0.0 {
		t.Errorf("expected FRI 0.0 for deterministic archetype, got %f", analysis.FRI)
	}

	if analysis.Archetype != "DETERMINISTIC" {
		t.Errorf("expected archetype DETERMINISTIC, got %s", analysis.Archetype)
	}

	if analysis.EntropyProfile.MaxNodeEntropy != 0.0 {
		t.Errorf("expected max node entropy 0.0, got %f", analysis.EntropyProfile.MaxNodeEntropy)
	}

	if len(analysis.EntropyProfile.BifurcationNodes) != 0 {
		t.Errorf("expected 0 bifurcation points, got %d", len(analysis.EntropyProfile.BifurcationNodes))
	}

	if !analysis.ThresholdPassed {
		t.Errorf("expected threshold to pass for deterministic runs")
	}
}

func TestBifurcatedEntropy(t *testing.T) {
	// Synthesize a perfectly balanced 50/50 split across 20 runs
	runs := make([]runner.Run, 20)
	now := time.Now()

	for i := 0; i < 20; i++ {
		var steps []runner.Step
		if i < 10 {
			// Branch A: tool_sql -> completion
			steps = append(steps, runner.Step{
				Index:      0,
				ActionType: "tool_call",
				ToolName:   "tool_sql",
				Timestamp:  now,
			})
			steps = append(steps, runner.Step{
				Index:       1,
				ActionType:  "completion",
				IsTerminal:  true,
				Observation: "sql result",
				Timestamp:   now,
			})
		} else {
			// Branch B: tool_vector -> completion
			steps = append(steps, runner.Step{
				Index:      0,
				ActionType: "tool_call",
				ToolName:   "tool_vector",
				Timestamp:  now,
			})
			steps = append(steps, runner.Step{
				Index:       1,
				ActionType:  "completion",
				IsTerminal:  true,
				Observation: "vector result",
				Timestamp:   now,
			})
		}

		runs[i] = runner.Run{
			ID:        "run",
			Iteration: i + 1,
			Steps:     steps,
			Success:   true,
		}
	}

	dag := entropy.BuildStateDAG(runs)
	entropyProf := entropy.ComputeDAGEntropy(dag)
	if entropyProf.MaxNodeEntropy <= 0 {
		t.Errorf("expected positive max node entropy, got %f", entropyProf.MaxNodeEntropy)
	}

	startNode := dag.Nodes["START"]
	if startNode == nil {
		t.Fatalf("missing START node")
	}

	// Shannon entropy of fair coin toss: - (0.5*log2(0.5) + 0.5*log2(0.5)) = 1.0 bit
	expectedEntropy := 1.0
	if math.Abs(startNode.Entropy-expectedEntropy) > 0.001 {
		t.Errorf("expected START node entropy 1.0, got %f", startNode.Entropy)
	}

	if !startNode.IsBifurcation {
		t.Errorf("expected START node to be flagged as bifurcation")
	}

	analysis := entropy.AnalyzeFlakiness(runs, 0.20, nil)
	if analysis.FRI < 0.20 {
		t.Errorf("expected FRI >= 0.20 for bifurcated agent, got %f", analysis.FRI)
	}

	if analysis.Archetype != "TOOL_BIFURCATION" {
		t.Errorf("expected archetype TOOL_BIFURCATION, got %s", analysis.Archetype)
	}
}
