package entropy

import (
	"math"

	"github.com/alexandrmotologa/flakeprobe/pkg/runner"
)

// TrajectoryDistanceProfile encapsulates sequence alignment and path divergence metrics.
type TrajectoryDistanceProfile struct {
	MeanNormalizedDistance float64 `json:"mean_normalized_distance"`
	MaxNormalizedDistance  float64 `json:"max_normalized_distance"`
	MinNormalizedDistance  float64 `json:"min_normalized_distance"`
	StepLengthMean         float64 `json:"step_length_mean"`
	StepLengthStdDev       float64 `json:"step_length_std_dev"`
	UniquePathCount        int     `json:"unique_path_count"`
}

// ComputeTrajectoryDistances evaluates pairwise Levenshtein differences between all runs.
func ComputeTrajectoryDistances(runs []runner.Run) *TrajectoryDistanceProfile {
	n := len(runs)
	if n <= 1 {
		return &TrajectoryDistanceProfile{
			UniquePathCount: n,
		}
	}

	sequences := make([][]string, n)
	uniquePaths := make(map[string]bool)
	var stepLengths []float64

	for i, r := range runs {
		seq := make([]string, 0, len(r.Steps))
		pathKey := ""
		for _, s := range r.Steps {
			token := s.ActionType
			if s.ActionType == "tool_call" && s.ToolName != "" {
				token = s.ToolName
			}
			seq = append(seq, token)
			pathKey += token + "->"
		}
		sequences[i] = seq
		uniquePaths[pathKey] = true
		stepLengths = append(stepLengths, float64(len(seq)))
	}

	totalNormDist := 0.0
	pairCount := 0
	maxDist := 0.0
	minDist := 1.0

	for i := 0; i < n-1; i++ {
		for j := i + 1; j < n; j++ {
			rawDist := levenshteinTokens(sequences[i], sequences[j])
			maxLen := math.Max(float64(len(sequences[i])), float64(len(sequences[j])))
			normDist := 0.0
			if maxLen > 0 {
				normDist = float64(rawDist) / maxLen
			}

			if normDist > maxDist {
				maxDist = normDist
			}
			if normDist < minDist {
				minDist = normDist
			}

			totalNormDist += normDist
			pairCount++
		}
	}

	meanDist := 0.0
	if pairCount > 0 {
		meanDist = totalNormDist / float64(pairCount)
	}

	// Calculate step length mean and standard deviation
	var sumLen float64
	for _, l := range stepLengths {
		sumLen += l
	}
	meanLen := sumLen / float64(n)

	var sumSqDiff float64
	for _, l := range stepLengths {
		diff := l - meanLen
		sumSqDiff += diff * diff
	}
	stdDevLen := math.Sqrt(sumSqDiff / float64(n))

	return &TrajectoryDistanceProfile{
		MeanNormalizedDistance: meanDist,
		MaxNormalizedDistance:  maxDist,
		MinNormalizedDistance:  minDist,
		StepLengthMean:         meanLen,
		StepLengthStdDev:       stdDevLen,
		UniquePathCount:        len(uniquePaths),
	}
}

// levenshteinTokens computes discrete Levenshtein edit distance over string token slices.
func levenshteinTokens(a, b []string) int {
	la := len(a)
	lb := len(b)

	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}

	dp := make([][]int, la+1)
	for i := range dp {
		dp[i] = make([]int, lb+1)
		dp[i][0] = i
	}
	for j := 0; j <= lb; j++ {
		dp[0][j] = j
	}

	for i := 1; i <= la; i++ {
		for j := 1; j <= lb; j++ {
			cost := 0
			if a[i-1] != b[j-1] {
				cost = 1
			}
			dp[i][j] = min3(
				dp[i-1][j]+1,      // deletion
				dp[i][j-1]+1,      // insertion
				dp[i-1][j-1]+cost, // substitution
			)
		}
	}

	return dp[la][lb]
}

func min3(a, b, c int) int {
	if a <= b && a <= c {
		return a
	}
	if b <= a && b <= c {
		return b
	}
	return c
}
