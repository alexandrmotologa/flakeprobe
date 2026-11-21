package test

import (
	"testing"

	"github.com/alexandrmotologa/flakeprobe/pkg/entropy"
	"github.com/alexandrmotologa/flakeprobe/pkg/mockagent"
	"github.com/alexandrmotologa/flakeprobe/pkg/runner"
	"github.com/alexandrmotologa/flakeprobe/pkg/suggest"
)

func TestSuggestMitigationsBifurcated(t *testing.T) {
	runs := make([]runner.Run, 10)
	for i := 0; i < 10; i++ {
		runs[i] = *mockagent.GenerateTrajectory(i+1, mockagent.ArchetypeBifurcated, "test")
	}

	analysis := entropy.AnalyzeFlakiness(runs, 0.20, nil)
	report := suggest.GenerateMitigations(analysis)

	if len(report.Directives) == 0 {
		t.Errorf("expected at least 1 mitigation directive for bifurcated archetype")
	}

	if report.SystemPromptPatchMarkdown == "" {
		t.Errorf("expected non-empty system prompt patch")
	}

	firstDirective := report.Directives[0]
	if firstDirective.SuggestedDirective == "" {
		t.Errorf("expected non-empty suggested directive")
	}
}

func TestSuggestMitigationsDeterministic(t *testing.T) {
	runs := make([]runner.Run, 5)
	for i := 0; i < 5; i++ {
		runs[i] = *mockagent.GenerateTrajectory(i+1, mockagent.ArchetypeDeterministic, "test")
	}

	analysis := entropy.AnalyzeFlakiness(runs, 0.20, nil)
	report := suggest.GenerateMitigations(analysis)

	if len(report.Directives) != 0 {
		t.Errorf("expected 0 directives for deterministic agent, got %d", len(report.Directives))
	}
}
