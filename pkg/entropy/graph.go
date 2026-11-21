package entropy

import (
	"fmt"
	"sort"
	"strings"

	"github.com/alexandrmotologa/flakeprobe/pkg/runner"
)

// GraphNode represents a discrete decision state in the agent trajectory DAG.
type GraphNode struct {
	ID             string                 `json:"id"`
	Label          string                 `json:"label"`
	ActionType     string                 `json:"action_type"`
	ToolName       string                 `json:"tool_name,omitempty"`
	StepIndex      int                    `json:"step_index"`
	Visits         int                    `json:"visits"`
	Entropy        float64                `json:"entropy"`
	NormalizedEnt  float64                `json:"normalized_entropy"`
	IsBifurcation  bool                   `json:"is_bifurcation"`
	IsTerminal     bool                   `json:"is_terminal"`
	AvgDurationMs  float64                `json:"avg_duration_ms"`
	SampleArgs     map[string]interface{} `json:"sample_args,omitempty"`
	OutgoingEdges  []*GraphEdge           `json:"outgoing_edges,omitempty"`
}

// GraphEdge represents a directed transition between two states.
type GraphEdge struct {
	Source      string  `json:"source"`
	Target      string  `json:"target"`
	Count       int     `json:"count"`
	Probability float64 `json:"probability"`
}

// StateDAG models the directed acyclic graph constructed from N agent runs.
type StateDAG struct {
	Nodes    map[string]*GraphNode `json:"nodes"`
	Edges    []*GraphEdge          `json:"edges"`
	RootID   string                `json:"root_id"`
	SinkIDs  []string              `json:"sink_ids"`
	NumRuns  int                   `json:"num_runs"`
	NumNodes int                   `json:"num_nodes"`
	NumEdges int                   `json:"num_edges"`
}

// BuildStateDAG constructs a transition graph from a slice of runs.
func BuildStateDAG(runs []runner.Run) *StateDAG {
	dag := &StateDAG{
		Nodes:   make(map[string]*GraphNode),
		Edges:   make([]*GraphEdge, 0),
		RootID:  "START",
		SinkIDs: make([]string, 0),
		NumRuns: len(runs),
	}

	dag.Nodes["START"] = &GraphNode{
		ID:         "START",
		Label:      "Start",
		ActionType: "start",
		StepIndex:  -1,
		Visits:     len(runs),
	}

	edgeCounts := make(map[string]int)
	nodeDurations := make(map[string][]int64)

	for _, run := range runs {
		prevNodeID := "START"

		for idx, step := range run.Steps {
			nodeID := canonicalNodeID(idx, step)
			node, exists := dag.Nodes[nodeID]
			if !exists {
				node = &GraphNode{
					ID:         nodeID,
					Label:      formatNodeLabel(step),
					ActionType: step.ActionType,
					ToolName:   step.ToolName,
					StepIndex:  idx,
					IsTerminal: step.IsTerminal,
					SampleArgs: step.Arguments,
				}
				dag.Nodes[nodeID] = node
			}
			node.Visits++
			nodeDurations[nodeID] = append(nodeDurations[nodeID], step.DurationMs)

			transitionKey := fmt.Sprintf("%s->%s", prevNodeID, nodeID)
			edgeCounts[transitionKey]++
			prevNodeID = nodeID
		}

		if prevNodeID != "START" {
			// Mark sink if not already marked
			isKnownSink := false
			for _, s := range dag.SinkIDs {
				if s == prevNodeID {
					isKnownSink = true
					break
				}
			}
			if !isKnownSink {
				dag.SinkIDs = append(dag.SinkIDs, prevNodeID)
			}
		}
	}

	// Calculate average durations
	for id, durs := range nodeDurations {
		if len(durs) > 0 {
			var total int64
			for _, d := range durs {
				total += d
			}
			dag.Nodes[id].AvgDurationMs = float64(total) / float64(len(durs))
		}
	}

	// Aggregate edge probabilities
	sourceTransitions := make(map[string]map[string]int)
	for key, count := range edgeCounts {
		parts := strings.Split(key, "->")
		src, dst := parts[0], parts[1]
		if sourceTransitions[src] == nil {
			sourceTransitions[src] = make(map[string]int)
		}
		sourceTransitions[src][dst] = count
	}

	for src, targets := range sourceTransitions {
		totalOut := 0
		for _, count := range targets {
			totalOut += count
		}

		for dst, count := range targets {
			prob := 0.0
			if totalOut > 0 {
				prob = float64(count) / float64(totalOut)
			}
			edge := &GraphEdge{
				Source:      src,
				Target:      dst,
				Count:       count,
				Probability: prob,
			}
			dag.Edges = append(dag.Edges, edge)
			dag.Nodes[src].OutgoingEdges = append(dag.Nodes[src].OutgoingEdges, edge)
		}
	}

	dag.NumNodes = len(dag.Nodes)
	dag.NumEdges = len(dag.Edges)

	return dag
}

func canonicalNodeID(stepIndex int, step runner.Step) string {
	if step.ActionType == "tool_call" && step.ToolName != "" {
		return fmt.Sprintf("step_%d:%s", stepIndex, step.ToolName)
	}
	if step.IsTerminal {
		return fmt.Sprintf("step_%d:TERMINAL", stepIndex)
	}
	return fmt.Sprintf("step_%d:%s", stepIndex, step.ActionType)
}

func formatNodeLabel(step runner.Step) string {
	if step.ActionType == "tool_call" && step.ToolName != "" {
		return fmt.Sprintf("call: %s", step.ToolName)
	}
	if step.IsTerminal {
		return "completion"
	}
	return step.ActionType
}

// GetSortedNodes returns nodes in topological step order.
func (g *StateDAG) GetSortedNodes() []*GraphNode {
	nodes := make([]*GraphNode, 0, len(g.Nodes))
	for _, n := range g.Nodes {
		nodes = append(nodes, n)
	}
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].StepIndex != nodes[j].StepIndex {
			return nodes[i].StepIndex < nodes[j].StepIndex
		}
		return nodes[i].ID < nodes[j].ID
	})
	return nodes
}
