package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/k9pranav/LLM_Cache/internal/cache"
	"github.com/k9pranav/LLM_Cache/internal/config"
	"github.com/k9pranav/LLM_Cache/internal/embedder"
	"github.com/k9pranav/LLM_Cache/internal/gateway"
	"github.com/k9pranav/LLM_Cache/internal/normalizer"
	"github.com/k9pranav/LLM_Cache/internal/policy"
	"github.com/k9pranav/LLM_Cache/internal/providers"
	llmrouter "github.com/k9pranav/LLM_Cache/internal/router"
)

func main() {
	ctx := context.Background()

	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatal("failed to load config:", err)
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

	mxbai := embedder.CreateEmbedder(
		cfg.Embedder.BaseURL,
		cfg.Embedder.Model,
	)

	stripper := normalizer.NewFillerStripper(
		cfg.Normalizer.FillerPhrases,
	)

	cachePolicy := policy.NewPolicy(
		cfg.Policy.MinResponseChar,
		cfg.Policy.MinTotalTokens,
		cfg.Policy.HedgingPhrases,
	)

	semanticCache := cache.NewSemanticCache(
		redisCache,
		mxbai,
		stripper,
		cfg.Cache.SimilarityThreshold,
		time.Duration(cfg.Cache.TTLSeconds)*time.Second,
		cachePolicy,
	)

	openAIProvider := providers.NewOpenAIProvider(
		cfg.LLM.APIKey,
		cfg.LLM.BaseURL,
		cfg.LLM.Model,
	)

	providerMap := map[string]providers.Provider{
		openAIProvider.Name(): openAIProvider,
	}

	llmRouter := llmrouter.NewLLMRouter(
		providerMap,
		cfg.LLM.Provider,
	)

	handler := gateway.NewHandler(
		semanticCache,
		llmRouter,
		cfg.LLM.Provider,
		cfg.LLM.Model,
	)

	httpRouter := gateway.NewRouter(
		handler,
		cfg.Health.Path,
		cfg.Metrics.Path,
	)

	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)

	server := &http.Server{
		Addr:         addr,
		Handler:      httpRouter,
		ReadTimeout:  time.Duration(cfg.Server.ReadTimeoutSeconds) * time.Second,
		WriteTimeout: time.Duration(cfg.Server.WriteTimeoutSeconds) * time.Second,
	}

	log.Println("LLM cache gateway listening on", addr)

	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
