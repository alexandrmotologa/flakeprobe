package suggest

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/alexandrmotologa/flakeprobe/pkg/entropy"
)

// MitigationPriority ranks the urgency of addressing a decision bifurcation.
type MitigationPriority string

const (
	PriorityCritical MitigationPriority = "CRITICAL" // H(s) >= 1.0
	PriorityHigh     MitigationPriority = "HIGH"     // 0.5 <= H(s) < 1.0
	PriorityMedium   MitigationPriority = "MEDIUM"   // Argument drift or lower entropy
)

// MitigationDirective contains an actionable prompt modification rule.
type MitigationDirective struct {
	NodeID             string             `json:"node_id"`
	StepIndex          int                `json:"step_index"`
	Priority           MitigationPriority `json:"priority"`
	CompetingTools     []string           `json:"competing_tools"`
	DiagnosticIssue    string             `json:"diagnostic_issue"`
	SuggestedDirective string             `json:"suggested_directive"`
}

// PromptMitigationReport summarizes all generated prompt guardrails.
type PromptMitigationReport struct {
	TargetArchetype           string                `json:"target_archetype"`
	Directives                []MitigationDirective `json:"directives"`
	SystemPromptPatchMarkdown string                `json:"system_prompt_patch_markdown"`
}

// GenerateMitigations inspects bifurcation nodes and argument drift to synthesize prompt fixes.
func GenerateMitigations(analysis *entropy.FlakinessAnalysis) *PromptMitigationReport {
	report := &PromptMitigationReport{
		TargetArchetype: analysis.Archetype,
		Directives:      make([]MitigationDirective, 0),
	}

	var patchLines []string
	patchLines = append(patchLines, "## Agent Decision & Tool Determinism Guardrails")

	// 1. Process Bifurcation Nodes
	for _, bp := range analysis.EntropyProfile.BifurcationNodes {
		var tools []string
		for _, b := range bp.BranchOptions {
			cleanTarget := strings.TrimPrefix(b.TargetNodeID, fmt.Sprintf("step_%d:", bp.StepIndex+1))
			cleanTarget = strings.TrimPrefix(cleanTarget, "step_0:")
			tools = append(tools, cleanTarget)
		}

		priority := PriorityHigh
		if bp.Entropy >= 1.0 {
			priority = PriorityCritical
		}

		competingStr := strings.Join(tools, " vs ")
		issue := fmt.Sprintf("Stochastic branching observed at step %d between %s (H = %.3f bits).", bp.StepIndex, competingStr, bp.Entropy)

		directiveText := ""
		if len(tools) >= 2 {
			directiveText = fmt.Sprintf("TOOL_PRIORITY_DIRECTIVE: When deciding between `%s` and `%s`, explicitly prioritize `%s` as primary; only invoke `%s` if primary tool returns an explicit error or missing data.",
				tools[0], tools[1], tools[0], tools[1])
		} else {
			directiveText = fmt.Sprintf("DECISION_DIRECTIVE: For step %d [%s], enforce deterministic transition to next canonical state.", bp.StepIndex, bp.Label)
		}

		directive := MitigationDirective{
			NodeID:             bp.NodeID,
			StepIndex:          bp.StepIndex,
			Priority:           priority,
			CompetingTools:     tools,
			DiagnosticIssue:    issue,
			SuggestedDirective: directiveText,
		}

		report.Directives = append(report.Directives, directive)
		patchLines = append(patchLines, fmt.Sprintf("- **Step %d Disambiguation**:\n  %s", bp.StepIndex, directiveText))
	}

	// 2. Process Argument Drift
	for _, ta := range analysis.ArgumentDrift.ToolAnalyses {
		if ta.DriftScore > 0.20 && len(ta.FluctuatingKeys) > 0 {
			keysStr := strings.Join(ta.FluctuatingKeys, ", ")
			directiveText := fmt.Sprintf("PARAMETER_STABILITY_DIRECTIVE: When calling tool `%s`, strictly enforce consistent argument values for [%s]. Do not vary schema parameters across repeated queries.",
				ta.ToolName, keysStr)

			directive := MitigationDirective{
				NodeID:             fmt.Sprintf("step_%d:%s", ta.StepIndex, ta.ToolName),
				StepIndex:          ta.StepIndex,
				Priority:           PriorityMedium,
				CompetingTools:     []string{ta.ToolName},
				DiagnosticIssue:    fmt.Sprintf("Parameter drift detected for tool '%s' on fields: %s", ta.ToolName, keysStr),
				SuggestedDirective: directiveText,
			}

			report.Directives = append(report.Directives, directive)
			patchLines = append(patchLines, fmt.Sprintf("- **Tool `%s` Schema Stability**:\n  %s", ta.ToolName, directiveText))
		}
	}

	// 3. Process Archetype Specifics (e.g. Cycle Trap)
	if analysis.Archetype == "CYCLE_TRAP" {
		directiveText := "LOOP_PREVENTION_DIRECTIVE: If any tool execution returns an error or empty result, do not re-attempt with identical parameters. Fail cleanly after a maximum of 1 retry."
		report.Directives = append(report.Directives, MitigationDirective{
			NodeID:             "GLOBAL_CYCLE_TRAP",
			StepIndex:          -1,
			Priority:           PriorityCritical,
			DiagnosticIssue:    "Agent repeatedly entered recursive retry loops.",
			SuggestedDirective: directiveText,
		})
		patchLines = append(patchLines, fmt.Sprintf("- **Global Retry Guardrail**:\n  %s", directiveText))
	}

	report.SystemPromptPatchMarkdown = strings.Join(patchLines, "\n\n")
	return report
}

// RenderTerminalSuggestions prints generated mitigation directives to stdout.
func RenderTerminalSuggestions(w io.Writer, rep *PromptMitigationReport) {
	if w == nil {
		w = os.Stdout
	}

	reset := "\033[0m"
	bold := "\033[1m"
	cyan := "\033[36m"
	yellow := "\033[33m"
	red := "\033[31m"
	green := "\033[32m"

	fmt.Fprintf(w, "\n%s%s=== flakeprobe: Automated Prompt Mitigation Directives ===%s\n", bold, cyan, reset)
	fmt.Fprintf(w, "Target Archetype: %s%s%s | Directives Generated: %d\n", bold, rep.TargetArchetype, reset, len(rep.Directives))
	fmt.Fprintln(w, strings.Repeat("-", 76))

	if len(rep.Directives) == 0 {
		fmt.Fprintf(w, "%sNo prompt modifications needed. Agent exhibits deterministic behavior.%s\n\n", green, reset)
		return
	}

	for idx, d := range rep.Directives {
		pColor := yellow
		if d.Priority == PriorityCritical {
			pColor = red
		}
		fmt.Fprintf(w, "[%d] %s[%s]%s %s (Step %d)\n", idx+1, pColor, d.Priority, reset, bold, d.StepIndex)
		fmt.Fprintf(w, "    Issue:     %s\n", d.DiagnosticIssue)
		fmt.Fprintf(w, "    Directive: %s%s%s\n\n", green, d.SuggestedDirective, reset)
	}

	fmt.Fprintf(w, "%sSuggested System Prompt Patch (Markdown):%s\n", bold, reset)
	fmt.Fprintln(w, rep.SystemPromptPatchMarkdown)
	fmt.Fprintln(w, "")
}
