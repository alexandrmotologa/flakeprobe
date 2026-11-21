package report

import (
	"fmt"
	"os"

	"github.com/alexandrmotologa/flakeprobe/pkg/entropy"
)

// ExportGitHubSummary appends an interactive Markdown table to the file specified by GITHUB_STEP_SUMMARY.
func ExportGitHubSummary(filePath string, analysis *entropy.FlakinessAnalysis, targetDesc string) error {
	if filePath == "" {
		filePath = os.Getenv("GITHUB_STEP_SUMMARY")
	}
	if filePath == "" {
		return nil
	}

	statusIcon := ":white_check_mark: PASSED"
	if analysis.IsUnstable {
		statusIcon = ":x: FAILED"
	}

	content := fmt.Sprintf(`### flakeprobe: Reliability & Path Entropy Report

| Evaluation Property | Value | Status |
| :--- | :--- | :--- |
| **Target Description** | %s | - |
| **Gate Result** | FRI %.4f (Threshold: %.2f) | %s |
| **Archetype** | `+"`%s`"+` | - |
| **Mean Trajectory Distance (D)** | %.4f | - |
| **Max Node Entropy H(s)** | %.4f bits | - |
| **Argument Drift Score** | %.2f | - |
| **Success Rate** | %.1f%% (%d runs) | - |
| **Unique Trajectories** | %d paths | - |

`,
		targetDesc,
		analysis.FRI,
		analysis.Threshold,
		statusIcon,
		analysis.Archetype,
		analysis.DistanceProfile.MeanNormalizedDistance,
		analysis.EntropyProfile.MaxNodeEntropy,
		analysis.ArgumentDrift.OverallDriftScore,
		analysis.SuccessRate*100.0,
		analysis.TotalRuns,
		analysis.DistanceProfile.UniquePathCount,
	)

	if len(analysis.EntropyProfile.BifurcationNodes) > 0 {
		content += "#### Decision Bifurcation Hotspots\n\n"
		content += "| Step | Node / Action | Entropy H(s) | Observed Branch Transitions |\n| :--- | :--- | :--- | :--- |\n"
		for _, bp := range analysis.EntropyProfile.BifurcationNodes {
			branches := ""
			for _, b := range bp.BranchOptions {
				branches += fmt.Sprintf("`%s` (%.0f%%) ", b.TargetNodeID, b.Probability*100.0)
			}
			content += fmt.Sprintf("| %d | `%s` | %.3f bits | %s |\n", bp.StepIndex, bp.Label, bp.Entropy, branches)
		}
		content += "\n"
	}

	f, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open GITHUB_STEP_SUMMARY file: %w", err)
	}
	defer f.Close()

	_, err = f.WriteString(content)
	return err
}
