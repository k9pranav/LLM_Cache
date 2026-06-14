package cache

import "github.com/k9pranav/LLM_Cache/pkg/types"

func BestMatch(candidates []types.CacheCandidate, threshold float64) types.CacheDecision {
	if len(candidates) == 0 {
		return types.CacheDecision{
			Hit:    false,
			Reason: "no semantic candidates found",
		}
	}

	best := candidates[0]

	if best.Similarity >= threshold {
		return types.CacheDecision{
			Hit:        true,
			HitType:    "semantic",
			Candidate:  &best,
			Reason:     "similarity above threshold",
			Similarity: best.Similarity,
		}
	}

	return types.CacheDecision{
		Hit:        false,
		Reason:     "best candidate below threshold",
		Similarity: best.Similarity,
	}
}
