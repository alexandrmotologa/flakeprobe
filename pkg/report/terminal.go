package report

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/alexandrmotologa/flakeprobe/pkg/entropy"
	"github.com/alexandrmotologa/flakeprobe/pkg/runner"
)

// ANSI color codes
const (
	reset     = "\033[0m"
	bold      = "\033[1m"
	dim       = "\033[2m"
	red       = "\033[31m"
	green     = "\033[32m"
	yellow    = "\033[33m"
	blue      = "\033[34m"
	magenta   = "\033[35m"
	cyan      = "\033[36m"
	brightRed = "\033[91m"
)

// RenderTerminal writes an ANSI-formatted diagnostic scorecard to stdout.
func RenderTerminal(w io.Writer, analysis *entropy.FlakinessAnalysis, runs []runner.Run, targetDesc string) {
	if w == nil {
		w = os.Stdout
	}

	friColor := green
	statusBadge := fmt.Sprintf("%s[PASS: DETERMINISTIC]%s", green, reset)
	if analysis.IsUnstable {
		friColor = brightRed
		statusBadge = fmt.Sprintf("%s[FAIL: UNSTABLE AGENT]%s", brightRed, reset)
	} else if analysis.FRI > 0.05 {
		friColor = yellow
		statusBadge = fmt.Sprintf("%s[WARN: MILD JITTER]%s", yellow, reset)
	}

	fmt.Fprintln(w, "")
	fmt.Fprintf(w, "%s%s=== flakeprobe: Agent Reliability & Path Entropy Report ===%s\n", bold, cyan, reset)
	fmt.Fprintf(w, "Target:     %s\n", targetDesc)
	fmt.Fprintf(w, "Status:     %s (Threshold: %.2f)\n", statusBadge, analysis.Threshold)
	fmt.Fprintf(w, "Archetype:  %s%s%s\n", bold, analysis.Archetype, reset)
	fmt.Fprintln(w, strings.Repeat("-", 68))

	// Metric Scorecard Table
	fmt.Fprintf(w, "%s%-32s %-16s %-16s%s\n", bold, "METRIC", "VALUE", "EVALUATION", reset)
	fmt.Fprintln(w, strings.Repeat("-", 68))

	friEval := "Stable"
	if analysis.FRI >= analysis.Threshold {
		friEval = "Exceeds Gate"
	}
	fmt.Fprintf(w, "%-32s %s%-16.4f%s %-16s\n", "Flakiness Risk Index (FRI)", friColor, analysis.FRI, reset, friEval)

	distEval := "Identical"
	if analysis.DistanceProfile.MeanNormalizedDistance > 0.3 {
		distEval = "Divergent"
	} else if analysis.DistanceProfile.MeanNormalizedDistance > 0 {
		distEval = "Minor Drift"
	}
	fmt.Fprintf(w, "%-32s %-16.4f %-16s\n", "Mean Trajectory Distance (D)", analysis.DistanceProfile.MeanNormalizedDistance, distEval)

	entEval := "Zero Entropy"
	if analysis.EntropyProfile.MaxNodeEntropy >= 1.0 {
		entEval = "High Volatility"
	} else if analysis.EntropyProfile.MaxNodeEntropy > 0 {
		entEval = "Bifurcated"
	}
	fmt.Fprintf(w, "%-32s %-16.4f %-16s\n", "Max Node Entropy H(s)", analysis.EntropyProfile.MaxNodeEntropy, entEval)

	fmt.Fprintf(w, "%-32s %-16.2f %-16s\n", "Argument Drift Score", analysis.ArgumentDrift.OverallDriftScore, "")
	if analysis.CostProfile != nil {
		fmt.Fprintf(w, "%-32s %-16.1f %-16s\n", "Mean Tokens per Run", analysis.CostProfile.MeanTokens, fmt.Sprintf("p95: %.0f", analysis.CostProfile.P95Tokens))
		costEval := "Uniform"
		if analysis.CostProfile.CostJitterRatio > 2.0 {
			costEval = "Runaway Risk"
		}
		fmt.Fprintf(w, "%-32s %-16s %-16s\n", "Cost Jitter Ratio", fmt.Sprintf("%.2fx", analysis.CostProfile.CostJitterRatio), costEval)
	}
	fmt.Fprintf(w, "%-32s %-16s %-16s\n", "Run Success Rate", fmt.Sprintf("%.1f%%", analysis.SuccessRate*100.0), "")
	fmt.Fprintf(w, "%-32s %-16d %-16s\n", "Unique Execution Paths", analysis.DistanceProfile.UniquePathCount, "")
	fmt.Fprintf(w, "%-32s %-16d %-16s\n", "Total Sampled Iterations", analysis.TotalRuns, "")
	fmt.Fprintln(w, strings.Repeat("-", 68))

	// Bifurcation Points Callout
	if len(analysis.EntropyProfile.BifurcationNodes) > 0 {
		fmt.Fprintf(w, "\n%s%sBifurcation Bottlenecks Detected (%d):%s\n", bold, yellow, len(analysis.EntropyProfile.BifurcationNodes), reset)
		for _, bp := range analysis.EntropyProfile.BifurcationNodes {
			fmt.Fprintf(w, "  * Step %d [%s] -> %sH(s) = %.3f bits%s\n", bp.StepIndex, bp.Label, bold, bp.Entropy, reset)
			for _, b := range bp.BranchOptions {
				fmt.Fprintf(w, "      |- %-28s (count: %2d, prob: %4.1f%%)\n", b.TargetNodeID, b.Count, b.Probability*100.0)
			}
		}
	} else {
		fmt.Fprintf(w, "\n%s%sNo decision bifurcations detected across all runs.%s\n", green, bold, reset)
	}

	// Recommendations
	if len(analysis.Recommendations) > 0 {
		fmt.Fprintf(w, "\n%sDiagnostics & Recommendations:%s\n", bold, reset)
		for _, rec := range analysis.Recommendations {
			fmt.Fprintf(w, "  - %s\n", rec)
		}
	}
	fmt.Fprintln(w, "")
}
