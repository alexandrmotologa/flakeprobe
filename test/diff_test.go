package test

import (
	"testing"

	"github.com/alexandrmotologa/flakeprobe/pkg/diff"
	"github.com/alexandrmotologa/flakeprobe/pkg/runner"
)

func TestCompareRunsIdentical(t *testing.T) {
	runA := runner.Run{
		ID: "run-001",
		Steps: []runner.Step{
			{Index: 0, ActionType: "tool_call", ToolName: "query_sql", Arguments: map[string]interface{}{"table": "users"}},
			{Index: 1, ActionType: "completion", IsTerminal: true},
		},
	}
	runB := runner.Run{
		ID: "run-002",
		Steps: []runner.Step{
			{Index: 0, ActionType: "tool_call", ToolName: "query_sql", Arguments: map[string]interface{}{"table": "users"}},
			{Index: 1, ActionType: "completion", IsTerminal: true},
		},
	}

	d := diff.CompareRuns(runA, runB)
	if !d.AreIdentical {
		t.Errorf("expected runs to be identical")
	}
	if d.DivergenceStep != -1 {
		t.Errorf("expected divergence step -1, got %d", d.DivergenceStep)
	}
}

func TestCompareRunsDivergent(t *testing.T) {
	runA := runner.Run{
		ID: "run-001",
		Steps: []runner.Step{
			{Index: 0, ActionType: "tool_call", ToolName: "query_sql"},
		},
	}
	runB := runner.Run{
		ID: "run-002",
		Steps: []runner.Step{
			{Index: 0, ActionType: "tool_call", ToolName: "vector_search"},
		},
	}

	d := diff.CompareRuns(runA, runB)
	if d.AreIdentical {
		t.Errorf("expected runs to diverge")
	}
	if d.DivergenceStep != 0 {
		t.Errorf("expected divergence at step 0, got %d", d.DivergenceStep)
	}
	if d.Steps[0].Status != diff.DiffStatusToolSubstitution {
		t.Errorf("expected tool substitution status, got %s", d.Steps[0].Status)
	}
}
