package cost

import (
	"math"
	"sort"
	"strings"

	"github.com/alexandrmotologa/flakeprobe/pkg/runner"
)

// ModelPricing defines costs per 1 million tokens for prompt and completion.
type ModelPricing struct {
	PromptPricePerM     float64 `json:"prompt_price_per_m"`
	CompletionPricePerM float64 `json:"completion_price_per_m"`
}

// DefaultPricingRegistry maps model prefixes to cost per 1M tokens in USD.
var DefaultPricingRegistry = map[string]ModelPricing{
	"gpt-4o": {
		PromptPricePerM:     2.50,
		CompletionPricePerM: 10.00,
	},
	"gpt-4o-mini": {
		PromptPricePerM:     0.15,
		CompletionPricePerM: 0.60,
	},
	"llama-3.3-70b": {
		PromptPricePerM:     0.35,
		CompletionPricePerM: 0.40,
	},
	"claude-3-5-sonnet": {
		PromptPricePerM:     3.00,
		CompletionPricePerM: 15.00,
	},
	"default": {
		PromptPricePerM:     0.50,
		CompletionPricePerM: 1.50,
	},
}

// GetPricing resolves model pricing from registry or default.
func GetPricing(modelName string, custom *ModelPricing) ModelPricing {
	if custom != nil {
		return *custom
	}
	lower := strings.ToLower(modelName)
	for k, v := range DefaultPricingRegistry {
		if strings.Contains(lower, k) {
			return v
		}
	}
	return DefaultPricingRegistry["default"]
}

// CostProfile summarizes token consumption variance and financial risk.
type CostProfile struct {
	ModelName       string   `json:"model_name"`
	TotalTokens     int      `json:"total_tokens"`
	MeanTokens      float64  `json:"mean_tokens"`
	MinTokens       int      `json:"min_tokens"`
	MaxTokens       int      `json:"max_tokens"`
	P95Tokens       float64  `json:"p95_tokens"`
	StdDevTokens    float64  `json:"std_dev_tokens"`
	MeanCostUSD     float64  `json:"mean_cost_usd"`
	MinCostUSD      float64  `json:"min_cost_usd"`
	MaxCostUSD      float64  `json:"max_cost_usd"`
	CostJitterRatio float64  `json:"cost_jitter_ratio"` // MaxCost / MinCost
	RunawayRuns     []string `json:"runaway_runs"`     // Runs with >2x mean tokens
}

// ComputeCostProfile evaluates token usage variance and cost volatility across runs.
func ComputeCostProfile(runs []runner.Run, modelName string, customPricing *ModelPricing) *CostProfile {
	n := len(runs)
	if n == 0 {
		return &CostProfile{ModelName: modelName}
	}

	pricing := GetPricing(modelName, customPricing)

	tokensList := make([]int, n)
	totalTokens := 0
	minTokens := runs[0].TotalTokens
	maxTokens := runs[0].TotalTokens

	for i, r := range runs {
		toks := r.TotalTokens
		// If total tokens not recorded directly on run, sum step estimates
		if toks == 0 {
			toks = len(r.Steps) * 120
		}
		tokensList[i] = toks
		totalTokens += toks
		if toks < minTokens {
			minTokens = toks
		}
		if toks > maxTokens {
			maxTokens = toks
		}
	}

	meanTokens := float64(totalTokens) / float64(n)

	// Calculate StdDev
	var sumSqDiff float64
	for _, tok := range tokensList {
		diff := float64(tok) - meanTokens
		sumSqDiff += diff * diff
	}
	stdDevTokens := math.Sqrt(sumSqDiff / float64(n))

	// Calculate P95
	sortedTokens := make([]int, n)
	copy(sortedTokens, tokensList)
	sort.Ints(sortedTokens)
	p95Idx := int(math.Ceil(0.95*float64(n))) - 1
	if p95Idx < 0 {
		p95Idx = 0
	}
	if p95Idx >= n {
		p95Idx = n - 1
	}
	p95Tokens := float64(sortedTokens[p95Idx])

	// Estimate costs (assuming roughly 65% prompt, 35% completion)
	calcCost := func(toks int) float64 {
		promptTokens := float64(toks) * 0.65
		completionTokens := float64(toks) * 0.35
		return (promptTokens/1_000_000.0)*pricing.PromptPricePerM +
			(completionTokens/1_000_000.0)*pricing.CompletionPricePerM
	}

	meanCost := calcCost(int(math.Round(meanTokens)))
	minCost := calcCost(minTokens)
	maxCost := calcCost(maxTokens)

	jitterRatio := 1.0
	if minCost > 0 {
		jitterRatio = maxCost / minCost
	}

	// Detect runaway runs (> 1.8x mean or > 2x min)
	var runawayRuns []string
	runawayThreshold := math.Max(meanTokens*1.8, float64(minTokens)*2.0)
	for _, r := range runs {
		toks := r.TotalTokens
		if toks == 0 {
			toks = len(r.Steps) * 120
		}
		if float64(toks) >= runawayThreshold && toks > minTokens {
			runawayRuns = append(runawayRuns, r.ID)
		}
	}

	return &CostProfile{
		ModelName:       modelName,
		TotalTokens:     totalTokens,
		MeanTokens:      meanTokens,
		MinTokens:       minTokens,
		MaxTokens:       maxTokens,
		P95Tokens:       p95Tokens,
		StdDevTokens:    stdDevTokens,
		MeanCostUSD:     meanCost,
		MinCostUSD:      minCost,
		MaxCostUSD:      maxCost,
		CostJitterRatio: jitterRatio,
		RunawayRuns:     runawayRuns,
	}
}
