package main

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/k9pranav/LLM_Cache/internal/config"
	"github.com/k9pranav/LLM_Cache/internal/policy"
	"github.com/k9pranav/LLM_Cache/internal/providers"
	"github.com/k9pranav/LLM_Cache/internal/router"
	"github.com/k9pranav/LLM_Cache/pkg/types"
)

func main() {
	ctx := context.Background()

	cfg, err := config.LoadConfig()

	if err != nil {
		log.Fatal("Failed to load config:", err)
	}

	if strings.TrimSpace(cfg.LLM.APIKey) == "" {
		log.Fatal("llm api_key is empty in config.local.yaml")
	}

	OpenAIProvider := providers.NewOpenAIProvider(
		cfg.LLM.APIKey, cfg.LLM.BaseURL, cfg.LLM.Model,
	)

	providerMap := map[string]providers.Provider{
		OpenAIProvider.Name(): OpenAIProvider,
	}

	llmRouter := router.NewLLMRouter(
		providerMap, cfg.LLM.Provider,
	)

	req := types.QueryRequest{
		TenantID: "tenant-a",
		Provider: cfg.LLM.Provider,
		Model:    cfg.LLM.Model,
		Messages: []types.Message{
			{
				Role:    "system",
				Content: "You are a helpful technical assistant.",
			},
			{
				Role:    "user",
				Content: "Explain Redis vector search in two sentences.",
			},
		},
		Temperature: 0.2,
	}

	resp, err := llmRouter.Route(ctx, req)
	if err != nil {
		log.Fatal("llm router failed:", err)
	}

	fmt.Println("===== MILESTONE 6 TEST =====")
	fmt.Println("Provider:", resp.Provider)
	fmt.Println("Model:", resp.Model)
	fmt.Println("Finish reason:", resp.FinishReason)
	fmt.Println("Prompt tokens:", resp.PromptTokens)
	fmt.Println("Completion tokens:", resp.CompletionTokens)
	fmt.Println("Total tokens:", resp.TotalTokens)
	fmt.Println()
	fmt.Println("Response:")
	fmt.Println(resp.Content)

	// redisCache := cache.NewRedisCache(
	// 	cfg.Redis.Addr,
	// 	cfg.Redis.Password,
	// 	cfg.Redis.DB,
	// )

	// if err := redisCache.Ping(ctx); err != nil {
	// 	log.Fatal("redis ping failed:", err)
	// }

	// if err := redisCache.CreateIndex(ctx, 1024); err != nil {
	// 	log.Fatal("failed to create redis vector index:", err)
	// }

	// //My embedder
	// mxbai := embedder.CreateEmbedder(
	// 	cfg.Embedder.BaseURL,
	// 	cfg.Embedder.Model,
	// )

	// stripper := normalizer.NewFillerStripper(cfg.Normalizer.FillerPhrases)

	// semanticCache := cache.NewSemanticCache(
	// 	redisCache, mxbai, stripper, cfg.Cache.SimilarityThreshold, time.Duration(cfg.Cache.TTLSeconds)*time.Second,
	// )

	// firstReq := types.QueryRequest{
	// 	TenantID: "tenant-a",
	// 	Provider: "fake-provider",
	// 	Model:    "fake-model",
	// 	Messages: []types.Message{
	// 		{Role: "system", Content: "You are a helpful technical assistant."},
	// 		{Role: "user", Content: "Explain Redis vector search."},
	// 	},
	// }

	// fakeLLMResp := types.LLMResponse{
	// 	Content:  "Redis vector search lets you find stored items meaning using embeddings",
	// 	Provider: "fake-provider",
	// 	Model:    "fake-model",
	// }

	// if err := semanticCache.Store(ctx, firstReq, fakeLLMResp); err != nil {
	// 	log.Fatal("failed to store cache entry:", err)
	// }

	// secondReq := types.QueryRequest{
	// 	TenantID: "tenant-a",
	// 	Provider: "fake-provider",
	// 	Model:    "fake-model",
	// 	Messages: []types.Message{
	// 		{Role: "system", Content: "You are a helpful technical assistant."},
	// 		{Role: "user", Content: "How does Redis vector database work?"},
	// 	},
	// }

	// resp, decision, err := semanticCache.Lookup(ctx, secondReq)

	// if err != nil {
	// 	log.Fatal("lookup failed:", err)
	// }

	// if decision.Hit {
	// 	fmt.Println("CACHE HIT")
	// 	fmt.Println("Hit type:", decision.HitType)
	// 	fmt.Println("Similarity:", decision.Similarity)
	// 	fmt.Println("Response:", resp.Content)
	// 	return
	// }

	// fmt.Println("CACHE MISS")
	// fmt.Println("Reason:", decision.Reason)

	cachePolicy := policy.NewPolicy(cfg.Policy.MinResponseChar, cfg.Policy.MinTotalTokens, cfg.Policy.HedgingPhrases)

	tests := []struct {
		name string
		resp types.LLMResponse
	}{
		{
			name: "good response",
			resp: types.LLMResponse{
				Content:      "Redis vector search stores embeddings and compares them using a vector similarity metric such as cosine distance.",
				Provider:     "fake-provider",
				Model:        "fake-model",
				TotalTokens:  20,
				FinishReason: "stop",
			},
		},
		{
			name: "too short response",
			resp: types.LLMResponse{
				Content:      "It depends.",
				Provider:     "fake-provider",
				Model:        "fake-model",
				TotalTokens:  2,
				FinishReason: "stop",
			},
		},
		{
			name: "hedging response",
			resp: types.LLMResponse{
				Content:      "assuming that, but Redis might use embeddings for vector search.",
				Provider:     "fake-provider",
				Model:        "fake-model",
				TotalTokens:  20,
				FinishReason: "stop",
			},
		},
		{
			name: "content filter response",
			resp: types.LLMResponse{
				Content:      "I cannot provide that answer.",
				Provider:     "fake-provider",
				Model:        "fake-model",
				TotalTokens:  20,
				FinishReason: "content_filter",
			},
		},
	}

	for _, test := range tests {
		shouldCache := cachePolicy.ShouldCache(test.resp)

		fmt.Println("Test:", test.name)
		fmt.Println("Response:", test.resp.Content)
		fmt.Println("Should cache:", shouldCache)
		fmt.Println()
	}

}
