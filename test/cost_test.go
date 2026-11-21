package test

import (
	"testing"

	"github.com/alexandrmotologa/flakeprobe/pkg/cost"
	"github.com/alexandrmotologa/flakeprobe/pkg/mockagent"
	"github.com/alexandrmotologa/flakeprobe/pkg/runner"
)

func TestCostProfileDeterministic(t *testing.T) {
	runs := make([]runner.Run, 5)
	for i := 0; i < 5; i++ {
		runs[i] = *mockagent.GenerateTrajectory(i+1, mockagent.ArchetypeDeterministic, "test")
	}

	profile := cost.ComputeCostProfile(runs, "gpt-4o-mini", nil)

	if profile.MeanTokens <= 0 {
		t.Errorf("expected positive mean tokens, got %f", profile.MeanTokens)
	}

	if profile.StdDevTokens != 0.0 {
		t.Errorf("expected 0.0 stddev tokens for deterministic agent, got %f", profile.StdDevTokens)
	}

	if profile.CostJitterRatio != 1.0 {
		t.Errorf("expected 1.0 jitter ratio for deterministic agent, got %f", profile.CostJitterRatio)
	}

	if len(profile.RunawayRuns) != 0 {
		t.Errorf("expected 0 runaway runs, got %d", len(profile.RunawayRuns))
	}
}

func TestCostProfileHallucinationLoop(t *testing.T) {
	runs := make([]runner.Run, 10)
	for i := 0; i < 10; i++ {
		runs[i] = *mockagent.GenerateTrajectory(i+1, mockagent.ArchetypeHallucinationLoop, "test")
	}

	profile := cost.ComputeCostProfile(runs, "gpt-4o", nil)

	if profile.CostJitterRatio < 1.0 {
		t.Errorf("expected jitter ratio >= 1.0, got %f", profile.CostJitterRatio)
	}

	if profile.MaxTokens <= profile.MinTokens {
		t.Errorf("expected max tokens > min tokens for loop archetype: min=%d, max=%d", profile.MinTokens, profile.MaxTokens)
	}
}
