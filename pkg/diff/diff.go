package diff

import (
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"

	"github.com/alexandrmotologa/flakeprobe/pkg/runner"
)

// StepDiffStatus represents the comparison classification of two aligned steps.
type StepDiffStatus string

const (
	DiffStatusMatch            StepDiffStatus = "MATCH"
	DiffStatusToolSubstitution StepDiffStatus = "TOOL_SUBSTITUTION"
	DiffStatusArgDiff          StepDiffStatus = "ARG_DIFF"
	DiffStatusInsertion        StepDiffStatus = "INSERTION"
	DiffStatusDeletion         StepDiffStatus = "DELETION"
)

// ArgFieldDiff represents a difference in a specific JSON parameter.
type ArgFieldDiff struct {
	Field string `json:"field"`
	ValA  string `json:"val_a"`
	ValB  string `json:"val_b"`
}

// StepDiff encapsulates side-by-side comparison of two trajectory steps.
type StepDiff struct {
	Index    int            `json:"index"`
	Status   StepDiffStatus `json:"status"`
	StepA    *runner.Step   `json:"step_a,omitempty"`
	StepB    *runner.Step   `json:"step_b,omitempty"`
	ArgDiffs []ArgFieldDiff `json:"arg_diffs,omitempty"`
}

// RunDiff details alignment and divergence between two specific execution runs.
type RunDiff struct {
	RunAID          string     `json:"run_a_id"`
	RunBID          string     `json:"run_b_id"`
	AreIdentical    bool       `json:"are_identical"`
	DivergenceStep  int        `json:"divergence_step"` // Step index of first divergence (-1 if identical)
	TokenDelta      int        `json:"token_delta"`
	DurationDeltaMs int64      `json:"duration_delta_ms"`
	Steps           []StepDiff `json:"steps"`
}

// CompareRuns aligns two trajectories and extracts step-by-step diffs.
func CompareRuns(runA, runB runner.Run) *RunDiff {
	diff := &RunDiff{
		RunAID:          runA.ID,
		RunBID:          runB.ID,
		AreIdentical:    true,
		DivergenceStep:  -1,
		TokenDelta:      runB.TotalTokens - runA.TotalTokens,
		DurationDeltaMs: runB.DurationMs - runA.DurationMs,
		Steps:           make([]StepDiff, 0),
	}

	lenA := len(runA.Steps)
	lenB := len(runB.Steps)
	maxLen := lenA
	if lenB > maxLen {
		maxLen = lenB
	}

	for i := 0; i < maxLen; i++ {
		sd := StepDiff{Index: i}

		if i >= lenA {
			// Step inserted in B
			diff.AreIdentical = false
			if diff.DivergenceStep == -1 {
				diff.DivergenceStep = i
			}
			sd.Status = DiffStatusInsertion
			sd.StepB = &runB.Steps[i]
			diff.Steps = append(diff.Steps, sd)
			continue
		}

		if i >= lenB {
			// Step deleted in B
			diff.AreIdentical = false
			if diff.DivergenceStep == -1 {
				diff.DivergenceStep = i
			}
			sd.Status = DiffStatusDeletion
			sd.StepA = &runA.Steps[i]
			diff.Steps = append(diff.Steps, sd)
			continue
		}

		sA := runA.Steps[i]
		sB := runB.Steps[i]
		sd.StepA = &sA
		sd.StepB = &sB

		// Compare tool / action
		nameA := sA.ActionType
		if sA.ToolName != "" {
			nameA = sA.ToolName
		}
		nameB := sB.ActionType
		if sB.ToolName != "" {
			nameB = sB.ToolName
		}

		if nameA != nameB {
			diff.AreIdentical = false
			if diff.DivergenceStep == -1 {
				diff.DivergenceStep = i
			}
			sd.Status = DiffStatusToolSubstitution
			diff.Steps = append(diff.Steps, sd)
			continue
		}

		// Compare arguments if tool call
		argDiffs := compareArgs(sA.Arguments, sB.Arguments)
		if len(argDiffs) > 0 {
			diff.AreIdentical = false
			if diff.DivergenceStep == -1 {
				diff.DivergenceStep = i
			}
			sd.Status = DiffStatusArgDiff
			sd.ArgDiffs = argDiffs
		} else {
			sd.Status = DiffStatusMatch
		}

		diff.Steps = append(diff.Steps, sd)
	}

	return diff
}

func compareArgs(argsA, argsB map[string]interface{}) []ArgFieldDiff {
	var diffs []ArgFieldDiff
	allKeys := make(map[string]bool)
	for k := range argsA {
		allKeys[k] = true
	}
	for k := range argsB {
		allKeys[k] = true
	}

	for k := range allKeys {
		valA, hasA := argsA[k]
		valB, hasB := argsB[k]

		if !hasA || !hasB || !reflect.DeepEqual(valA, valB) {
			strA := "<missing>"
			strB := "<missing>"
			if hasA {
				strA = fmt.Sprintf("%v", valA)
			}
			if hasB {
				strB = fmt.Sprintf("%v", valB)
			}
			diffs = append(diffs, ArgFieldDiff{
				Field: k,
				ValA:  strA,
				ValB:  strB,
			})
		}
	}
	return diffs
}

// RenderTerminalDiff outputs a formatted terminal comparison between two runs.
func RenderTerminalDiff(w io.Writer, diff *RunDiff) {
	if w == nil {
		w = os.Stdout
	}

	reset := "\033[0m"
	bold := "\033[1m"
	green := "\033[32m"
	red := "\033[31m"
	yellow := "\033[33m"
	cyan := "\033[36m"

	fmt.Fprintf(w, "\n%s%s=== flakeprobe Trajectory Diff: %s vs %s ===%s\n", bold, cyan, diff.RunAID, diff.RunBID, reset)
	if diff.AreIdentical {
		fmt.Fprintf(w, "Status: %sIDENTICAL TRAJECTORIES%s\n", green, reset)
	} else {
		fmt.Fprintf(w, "Status: %sDIVERGENT%s (First divergence at Step %d)\n", red, reset, diff.DivergenceStep)
	}
	fmt.Fprintf(w, "Token Delta: %d | Duration Delta: %dms\n", diff.TokenDelta, diff.DurationDeltaMs)
	fmt.Fprintln(w, strings.Repeat("-", 76))
	fmt.Fprintf(w, "%-6s %-18s %-24s %-24s\n", "STEP", "ALIGNMENT", diff.RunAID, diff.RunBID)
	fmt.Fprintln(w, strings.Repeat("-", 76))

	for _, s := range diff.Steps {
		labelA := "-"
		labelB := "-"
		if s.StepA != nil {
			labelA = s.StepA.ActionType
			if s.StepA.ToolName != "" {
				labelA = s.StepA.ToolName
			}
		}
		if s.StepB != nil {
			labelB = s.StepB.ActionType
			if s.StepB.ToolName != "" {
				labelB = s.StepB.ToolName
			}
		}

		statusColor := green
		switch s.Status {
		case DiffStatusMatch:
			statusColor = green
		case DiffStatusArgDiff:
			statusColor = yellow
		case DiffStatusToolSubstitution, DiffStatusInsertion, DiffStatusDeletion:
			statusColor = red
		}

		fmt.Fprintf(w, "%-6d %s%-18s%s %-24s %-24s\n", s.Index, statusColor, s.Status, reset, labelA, labelB)
		for _, ad := range s.ArgDiffs {
			fmt.Fprintf(w, "       |-> Arg '%s': [%s] != [%s]\n", ad.Field, ad.ValA, ad.ValB)
		}
	}
	fmt.Fprintln(w, "")
}
