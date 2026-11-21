package entropy

import (
	"math"

	"github.com/alexandrmotologa/flakeprobe/pkg/cost"
	"github.com/alexandrmotologa/flakeprobe/pkg/runner"
)

// FlakinessWeights controls contribution of each component to the composite FRI score.
type FlakinessWeights struct {
	DistanceWeight float64 `json:"distance_weight"`
	OutcomeWeight  float64 `json:"outcome_weight"`
	EntropyWeight  float64 `json:"entropy_weight"`
}

// DefaultWeights returns calibrated weights for the Flakiness Risk Index.
func DefaultWeights() FlakinessWeights {
	return FlakinessWeights{
		DistanceWeight: 0.40,
		OutcomeWeight:  0.35,
		EntropyWeight:  0.25,
	}
}

// FlakinessAnalysis contains the complete statistical diagnosis for a test campaign.
type FlakinessAnalysis struct {
	FRI                 float64                    `json:"fri"` // 0.0 to 1.0
	FRIPercent          float64                    `json:"fri_percent"` // 0% to 100%
	Archetype           string                     `json:"archetype"`
	IsUnstable          bool                       `json:"is_unstable"`
	ThresholdPassed     bool                       `json:"threshold_passed"`
	Threshold           float64                    `json:"threshold"`
	TotalRuns           int                        `json:"total_runs"`
	SuccessRate         float64                    `json:"success_rate"`
	DistanceProfile     TrajectoryDistanceProfile  `json:"distance_profile"`
	EntropyProfile      EntropyProfile             `json:"entropy_profile"`
	ArgumentDrift       ArgumentDriftProfile       `json:"argument_drift"`
	StateDAG            *StateDAG                  `json:"state_dag"`
	PrimaryBifurcation  *BifurcationPoint          `json:"primary_bifurcation,omitempty"`
	CostProfile         *cost.CostProfile          `json:"cost_profile,omitempty"`
	Recommendations     []string                   `json:"recommendations"`
}

// AnalyzeFlakiness runs the complete statistical pipeline and computes the Flakiness Risk Index.
func AnalyzeFlakiness(runs []runner.Run, threshold float64, weights *FlakinessWeights) *FlakinessAnalysis {
	if weights == nil {
		w := DefaultWeights()
		weights = &w
	}

	dag := BuildStateDAG(runs)
	entropyProf := ComputeDAGEntropy(dag)
	distProf := ComputeTrajectoryDistances(runs)
	argDrift := ComputeArgumentDrift(runs, nil)

	n := len(runs)
	successCount := 0
	for _, r := range runs {
		if r.Success {
			successCount++
		}
	}

	successRate := 0.0
	outcomeVariation := 0.0
	if n > 0 {
		successRate = float64(successCount) / float64(n)
		failureCount := n - successCount
		// Outcome variation is maximized (1.0) when successes == failures (50/50 split), 0.0 when 100% or 0%
		outcomeVariation = 1.0 - (math.Abs(float64(successCount-failureCount)) / float64(n))
	}

	// Normalized entropy factor
	normEntropy := 0.0
	if entropyProf.MaxNodeEntropy > 0 {
		// Log2 of unique outgoing targets or max 4
		normEntropy = math.Min(1.0, entropyProf.MaxNodeEntropy/2.0)
	}

	// Composite FRI: w1 * dist + w2 * outcome + w3 * entropy
	rawFRI := (weights.DistanceWeight * distProf.MeanNormalizedDistance) +
		(weights.OutcomeWeight * outcomeVariation) +
		(weights.EntropyWeight * normEntropy)

	// Clamp to [0.0, 1.0]
	fri := math.Max(0.0, math.Min(1.0, rawFRI))

	// Find primary bifurcation point (highest entropy)
	var primaryBifurcation *BifurcationPoint
	maxH := 0.0
	for i := range entropyProf.BifurcationNodes {
		bp := &entropyProf.BifurcationNodes[i]
		if bp.Entropy > maxH {
			maxH = bp.Entropy
			primaryBifurcation = bp
		}
	}

	// Classify archetype
	archetype := "DETERMINISTIC"
	var recommendations []string

	if fri < 0.02 && argDrift.OverallDriftScore < 0.05 {
		archetype = "DETERMINISTIC"
		recommendations = append(recommendations, "Agent execution is fully deterministic across all runs.")
	} else if distProf.MeanNormalizedDistance < 0.05 && argDrift.OverallDriftScore > 0.10 {
		archetype = "ARGUMENT_DRIFT"
		recommendations = append(recommendations, "Tool calling sequence is stable, but parameter arguments vary across runs. Check schema constraints or prompt formatting.")
	} else if outcomeVariation > 0.20 {
		archetype = "OUTCOME_DISCORDANCE"
		recommendations = append(recommendations, "Execution yields conflicting exit states (success vs failure). Investigate brittle tool handlers or unhandled exceptions.")
	} else if distProf.StepLengthStdDev > 1.5 {
		archetype = "CYCLE_TRAP"
		recommendations = append(recommendations, "High step length variance indicates agent gets caught in iterative retry or hallucination loops.")
	} else if entropyProf.MaxNodeEntropy >= 0.5 {
		archetype = "TOOL_BIFURCATION"
		recommendations = append(recommendations, "Agent branches into divergent tool pathways at decision points. Add explicit disambiguation instructions in system prompt.")
	} else {
		archetype = "STOCHASTIC_DRIFT"
		recommendations = append(recommendations, "Minor stochastic variance detected across trajectories.")
	}

	if threshold <= 0 {
		threshold = 0.20
	}

	isUnstable := fri >= threshold
	thresholdPassed := !isUnstable

	costProfile := cost.ComputeCostProfile(runs, "default", nil)
	if costProfile.CostJitterRatio > 2.0 {
		recommendations = append(recommendations, "High token cost variance detected across trajectories. Some runs consume excessive iterations.")
	}

	return &FlakinessAnalysis{
		FRI:                fri,
		FRIPercent:         fri * 100.0,
		Archetype:          archetype,
		IsUnstable:         isUnstable,
		ThresholdPassed:    thresholdPassed,
		Threshold:          threshold,
		TotalRuns:          n,
		SuccessRate:        successRate,
		DistanceProfile:    *distProf,
		EntropyProfile:     *entropyProf,
		ArgumentDrift:      *argDrift,
		StateDAG:           dag,
		PrimaryBifurcation: primaryBifurcation,
		CostProfile:        costProfile,
		Recommendations:    recommendations,
	}
}
