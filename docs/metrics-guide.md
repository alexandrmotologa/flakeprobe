# Metrics and Mathematical Foundations

This guide outlines the mathematical formulas used by flakeprobe to quantify agent non-determinism.

## 1. State Transition Probabilities

Let an execution run $r \in \{1, \dots, N\}$ be a sequence of discrete actions $A^{(r)} = (a_1^{(r)}, \dots, a_{T_r}^{(r)})$.
For state $s$, the empirical probability of transitioning to state $s'$ is:

$$P(s' \mid s) = \frac{\sum_{r=1}^N \sum_{t=1}^{T_r - 1} \mathbf{1}(a_t^{(r)} = s \land a_{t+1}^{(r)} = s')}{\sum_{r=1}^N \sum_{t=1}^{T_r} \mathbf{1}(a_t^{(r)} = s)}$$

## 2. Shannon Decision Node Entropy ($H(s)$)

Entropy measures the uncertainty of transitions from node $s$:

$$H(s) = -\sum_{s'} P(s' \mid s) \log_2 P(s' \mid s)$$

- $H(s) = 0.0$: Node is deterministic across all runs.
- $0.0 < H(s) < 1.0$: Node exhibits minor branching.
- $H(s) \ge 1.0$: Node represents a significant bifurcation point.

## 3. Normalized Trajectory Distance ($\bar{D}$)

Given action sequences $A^{(i)}$ and $A^{(j)}$, normalized edit distance is:

$$\bar{D} = \frac{2}{N(N - 1)} \sum_{i < j} \frac{\text{Levenshtein}(A^{(i)}, A^{(j)})}{\max(|A^{(i)}|, |A^{(j)}|)}$$

- $\bar{D} = 0.0$: All runs took identical tool sequences.
- $\bar{D} > 0.3$: Paths diverge considerably across runs.

## 4. Flakiness Risk Index (FRI)

Composite metric combining trajectory distance, outcome variation, and maximum node entropy:

$$\text{FRI} = w_1 \bar{D} + w_2 \left(1 - \frac{|S - F|}{N}\right) + w_3 \frac{\max_s H(s)}{\log_2 |\mathcal{S}_{\text{out}}| + \epsilon}$$

Where $S$ is successes, $F$ is failures, and default weights are:
- $w_1 = 0.40$ (Trajectory distance)
- $w_2 = 0.35$ (Outcome discordance)
- $w_3 = 0.25$ (Maximum node entropy)

An FRI value exceeding the configured threshold (default 0.20) signals an unstable agent.
