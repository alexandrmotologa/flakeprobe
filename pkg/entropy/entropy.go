package entropy

import (
	"math"

	"github.com/alexandrmotologa/flakeprobe/pkg/runner"
)

// EntropyProfile summarizes entropy statistics across the entire state graph.
type EntropyProfile struct {
	MaxNodeEntropy   float64            `json:"max_node_entropy"`
	MeanNodeEntropy  float64            `json:"mean_node_entropy"`
	BifurcationNodes []BifurcationPoint `json:"bifurcation_nodes"`
	NodeEntropyMap   map[string]float64 `json:"node_entropy_map"`
}

// BifurcationPoint details a specific node where the agent branches non-deterministically.
type BifurcationPoint struct {
	NodeID        string             `json:"node_id"`
	Label         string             `json:"label"`
	StepIndex     int                `json:"step_index"`
	Entropy       float64            `json:"entropy"`
	Normalized    float64            `json:"normalized"`
	BranchOptions []BranchTransition `json:"branch_options"`
}

// BranchTransition details one of the alternative branches from a bifurcation point.
type BranchTransition struct {
	TargetNodeID string  `json:"target_node_id"`
	Count        int     `json:"count"`
	Probability  float64 `json:"probability"`
}

// ComputeDAGEntropy calculates Shannon entropy for all nodes in the state graph.
func ComputeDAGEntropy(dag *StateDAG) *EntropyProfile {
	profile := &EntropyProfile{
		BifurcationNodes: make([]BifurcationPoint, 0),
		NodeEntropyMap:   make(map[string]float64),
	}

	totalEntropy := 0.0
	evaluatedNodes := 0

	for _, node := range dag.Nodes {
		if len(node.OutgoingEdges) <= 1 {
			node.Entropy = 0.0
			node.NormalizedEnt = 0.0
			node.IsBifurcation = false
			profile.NodeEntropyMap[node.ID] = 0.0
			continue
		}

		entropy := 0.0
		numBranches := len(node.OutgoingEdges)

		for _, edge := range node.OutgoingEdges {
			p := edge.Probability
			if p > 0 {
				entropy -= p * math.Log2(p)
			}
		}

		// Edge case: clamp numerical imprecision
		if entropy < 0 {
			entropy = 0
		}

		node.Entropy = entropy
		profile.NodeEntropyMap[node.ID] = entropy

		maxPossibleEntropy := math.Log2(float64(numBranches))
		if maxPossibleEntropy > 0 {
			node.NormalizedEnt = entropy / maxPossibleEntropy
		} else {
			node.NormalizedEnt = 0.0
		}

		if entropy > 0.001 {
			node.IsBifurcation = true

			bp := BifurcationPoint{
				NodeID:        node.ID,
				Label:         node.Label,
				StepIndex:     node.StepIndex,
				Entropy:       entropy,
				Normalized:    node.NormalizedEnt,
				BranchOptions: make([]BranchTransition, 0, len(node.OutgoingEdges)),
			}

			for _, edge := range node.OutgoingEdges {
				bp.BranchOptions = append(bp.BranchOptions, BranchTransition{
					TargetNodeID: edge.Target,
					Count:        edge.Count,
					Probability:  edge.Probability,
				})
			}

			profile.BifurcationNodes = append(profile.BifurcationNodes, bp)
		}

		if entropy > profile.MaxNodeEntropy {
			profile.MaxNodeEntropy = entropy
		}

		totalEntropy += entropy
		evaluatedNodes++
	}

	if evaluatedNodes > 0 {
		profile.MeanNodeEntropy = totalEntropy / float64(evaluatedNodes)
	}

	return profile
}

// CalculateTrajectoryEntropy computes the cumulative and average entropy along a run's path.
func CalculateTrajectoryEntropy(run runner.Run, profile *EntropyProfile) (float64, float64) {
	if len(run.Steps) == 0 {
		return 0, 0
	}

	var sum float64
	count := 0

	// Check start node
	if h, ok := profile.NodeEntropyMap["START"]; ok && h > 0 {
		sum += h
		count++
	}

	for idx, step := range run.Steps {
		nodeID := canonicalNodeID(idx, step)
		if h, ok := profile.NodeEntropyMap[nodeID]; ok {
			sum += h
			count++
		}
	}

	if count == 0 {
		return 0, 0
	}
	return sum, sum / float64(count)
}
