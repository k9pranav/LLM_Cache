package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/k9pranav/LLM_Cache/internal/cache"
	"github.com/k9pranav/LLM_Cache/internal/config"
	"github.com/k9pranav/LLM_Cache/internal/embedder"
	"github.com/k9pranav/LLM_Cache/internal/normalizer"
	"github.com/k9pranav/LLM_Cache/pkg/types"
)

func main() {
	ctx := context.Background()

	cfg, err := config.LoadConfig()

	if err != nil {
		log.Fatal("Failed to load config:", err)
	}

	redisCache := cache.NewRedisCache(
		cfg.Redis.Addr,
		cfg.Redis.Password,
		cfg.Redis.DB,
	)

	if err := redisCache.Ping(ctx); err != nil {
		log.Fatal("redis ping failed:", err)
	}

	if err := redisCache.CreateIndex(ctx, 1024); err != nil {
		log.Fatal("failed to create redis vector index:", err)
	}

	//My embedder
	mxbai := embedder.CreateEmbedder(
		cfg.Embedder.BaseURL,
		cfg.Embedder.Model,
	)

	stripper := normalizer.NewFillerStripper(cfg.Normalizer.FillerPhrases)

	semanticCache := cache.NewSemanticCache(
		redisCache, mxbai, stripper, cfg.Cache.SimilarityThreshold, time.Duration(cfg.Cache.TTLSeconds)*time.Second,
	)

	firstReq := types.QueryRequest{
		TenantID: "tenant-a",
		Provider: "fake-provider",
		Model:    "fake-model",
		Messages: []types.Message{
			{Role: "system", Content: "You are a helpful technical assistant."},
			{Role: "user", Content: "Explain Redis vector search."},
		},
	}

	fakeLLMResp := types.LLMResponse{
		Content:  "Redis vector search lets you find stored items meaning using embeddings",
		Provider: "fake-provider",
		Model:    "fake-model",
	}

	if err := semanticCache.Store(ctx, firstReq, fakeLLMResp); err != nil {
		log.Fatal("failed to store cache entry:", err)
	}

	secondReq := types.QueryRequest{
		TenantID: "tenant-a",
		Provider: "fake-provider",
		Model:    "fake-model",
		Messages: []types.Message{
			{Role: "system", Content: "You are a helpful technical assistant."},
			{Role: "user", Content: "How does Redis vector database work?"},
		},
	}

	resp, decision, err := semanticCache.Lookup(ctx, secondReq)

	if err != nil {
		log.Fatal("lookup failed:", err)
	}

	if decision.Hit {
		fmt.Println("CACHE HIT")
		fmt.Println("Hit type:", decision.HitType)
		fmt.Println("Similarity:", decision.Similarity)
		fmt.Println("Response:", resp.Content)
		return
	}

	fmt.Println("CACHE MISS")
	fmt.Println("Reason:", decision.Reason)

}
