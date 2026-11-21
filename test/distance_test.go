package test

import (
	"testing"

	"github.com/alexandrmotologa/flakeprobe/pkg/entropy"
	"github.com/alexandrmotologa/flakeprobe/pkg/runner"
)

func TestTrajectoryDistanceIdentical(t *testing.T) {
	runs := []runner.Run{
		{
			ID: "run-1",
			Steps: []runner.Step{
				{ActionType: "tool_call", ToolName: "tool_a"},
				{ActionType: "tool_call", ToolName: "tool_b"},
				{ActionType: "completion", IsTerminal: true},
			},
		},
		{
			ID: "run-2",
			Steps: []runner.Step{
				{ActionType: "tool_call", ToolName: "tool_a"},
				{ActionType: "tool_call", ToolName: "tool_b"},
				{ActionType: "completion", IsTerminal: true},
			},
		},
	}

	distProf := entropy.ComputeTrajectoryDistances(runs)

	if distProf.MeanNormalizedDistance != 0.0 {
		t.Errorf("expected 0.0 distance for identical trajectories, got %f", distProf.MeanNormalizedDistance)
	}

	if distProf.UniquePathCount != 1 {
		t.Errorf("expected 1 unique path, got %d", distProf.UniquePathCount)
	}
}

func TestTrajectoryDistanceCompletelyDivergent(t *testing.T) {
	runs := []runner.Run{
		{
			ID: "run-1",
			Steps: []runner.Step{
				{ActionType: "tool_call", ToolName: "tool_a"},
				{ActionType: "tool_call", ToolName: "tool_b"},
			},
		},
		{
			ID: "run-2",
			Steps: []runner.Step{
				{ActionType: "tool_call", ToolName: "tool_x"},
				{ActionType: "tool_call", ToolName: "tool_y"},
			},
		},
	}

	distProf := entropy.ComputeTrajectoryDistances(runs)

	// Completely replaced 2 tokens out of 2: normalized distance = 2 / 2 = 1.0
	if distProf.MeanNormalizedDistance != 1.0 {
		t.Errorf("expected 1.0 normalized distance, got %f", distProf.MeanNormalizedDistance)
	}

	if distProf.UniquePathCount != 2 {
		t.Errorf("expected 2 unique paths, got %d", distProf.UniquePathCount)
	}
}
