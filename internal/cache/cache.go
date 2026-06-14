package cache

import (
	"context"
	"errors"
	"time"

	"github.com/k9pranav/LLM_Cache/internal/conversation"
	"github.com/k9pranav/LLM_Cache/internal/normalizer"
	"github.com/k9pranav/LLM_Cache/pkg/types"
	"github.com/redis/go-redis/v9"
)

type TextEmbedder interface {
	Embed(text string) ([]float64, error)
}

type SemanticCache struct {
	Redis               *RedisCache
	Embedder            TextEmbedder
	Stripper            *normalizer.FillerStripper
	SimilarityThreshold float64
	TTL                 time.Duration
	TopK                int
	LastNMessages       int
}

func NewSemanticCache(
	redisCache *RedisCache,
	embedder TextEmbedder,
	stripper *normalizer.FillerStripper,
	similarityThreshold float64,
	ttl time.Duration,
) *SemanticCache {
	return &SemanticCache{
		Redis:               redisCache,
		Embedder:            embedder,
		Stripper:            stripper,
		SimilarityThreshold: similarityThreshold,
		TTL:                 ttl,
		TopK:                5,
		LastNMessages:       4,
	}
}

// Lookup tries exact cache first, then semantic vector cache.
func (c *SemanticCache) Lookup(
	ctx context.Context,
	req types.QueryRequest,
) (*types.LLMResponse, types.CacheDecision, error) {
	nq := conversation.BuildLastNContext(req.Messages, c.Stripper, c.LastNMessages)

	exactKey := ExactKey(req.TenantID, nq.ExactKeyText, req.Model, nq.SystemPromptHash)

	exactEntry, err := c.Redis.GetExactEntry(ctx, exactKey)
	if err == nil {
		_ = c.Redis.IncrementReuseCount(ctx, exactEntry.ID)

		return cacheEntryToResponse(exactEntry), types.CacheDecision{
			Hit:        true,
			HitType:    "exact",
			Reason:     "exact contextual key matched",
			Similarity: 1.0,
		}, nil
	}

	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, types.CacheDecision{Hit: false, Reason: "exact lookup failed"}, err
	}

	vector64, err := c.Embedder.Embed(nq.SemanticText)
	if err != nil {
		return nil, types.CacheDecision{Hit: false, Reason: "embedding failed"}, err
	}

	vector32 := float64ToFloat32(vector64)

	lookupReq := types.SemanticLookupRequest{
		TenantID:  req.TenantID,
		Vector:    vector32,
		TopK:      c.TopK,
		Threshold: c.SimilarityThreshold,
		Filters: map[string]string{
			"provider":           req.Provider,
			"model":              req.Model,
			"system_prompt_hash": nq.SystemPromptHash,
		},
	}

	candidates, err := c.Redis.SemanticSearch(ctx, lookupReq)
	if err != nil {
		return nil, types.CacheDecision{Hit: false, Reason: "semantic lookup failed"}, err
	}

	decision := BestMatch(candidates, c.SimilarityThreshold)
	if !decision.Hit {
		return nil, decision, nil
	}

	_ = c.Redis.IncrementReuseCount(ctx, decision.Candidate.Entry.ID)

	return cacheEntryToResponse(&decision.Candidate.Entry), decision, nil
}

// Store saves a new LLM response into exact + semantic cache.
func (c *SemanticCache) Store(
	ctx context.Context,
	req types.QueryRequest,
	resp types.LLMResponse,
) error {
	nq := conversation.BuildLastNContext(req.Messages, c.Stripper, c.LastNMessages)

	vector64, err := c.Embedder.Embed(nq.SemanticText)
	if err != nil {
		return err
	}

	vector32 := float64ToFloat32(vector64)

	now := time.Now().UTC()
	expiresAt := now.Add(c.TTL)

	entryID := EntryKey(req.TenantID, nq.ExactKeyText, req.Model, nq.SystemPromptHash)
	exactKey := ExactKey(req.TenantID, nq.ExactKeyText, req.Model, nq.SystemPromptHash)

	entry := types.CacheEntry{
		ID:               entryID,
		TenantID:         req.TenantID,
		Scope:            "tenant",
		RawQuery:         latestUserMessage(req.Messages),
		NormalizedQuery:  nq.SemanticText,
		Response:         resp.Content,
		Provider:         resp.Provider,
		Model:            resp.Model,
		SystemPromptHash: nq.SystemPromptHash,
		CreatedAt:        now,
		ExpiresAt:        expiresAt,
		ReuseCount:       0,
	}

	return c.Redis.StoreEntry(ctx, entry, vector32, exactKey, c.TTL)
}

func cacheEntryToResponse(entry *types.CacheEntry) *types.LLMResponse {
	return &types.LLMResponse{
		Content:  entry.Response,
		Provider: entry.Provider,
		Model:    entry.Model,
	}
}

func latestUserMessage(messages []types.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			return messages[i].Content
		}
	}

	return ""
}

func float64ToFloat32(input []float64) []float32 {
	output := make([]float32, len(input))

	for i, value := range input {
		output[i] = float32(value)
	}

	return output
}
