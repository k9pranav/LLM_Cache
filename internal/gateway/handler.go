package gateway

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/k9pranav/LLM_Cache/internal/cache"
	llmrouter "github.com/k9pranav/LLM_Cache/internal/router"
	"github.com/k9pranav/LLM_Cache/pkg/types"
)

type GatewayStats struct {
	HTTPRequestsTotal int64
	CacheHits         int64
	CacheMisses       int64
	LLMRequests       int64
	AsyncStoreErrors  int64
}

type Handler struct {
	Cache           *cache.SemanticCache
	LLMRouter       *llmrouter.LLMRouter
	DefaultProvider string
	DefaultModel    string
	StartedAt       time.Time
	Stats           *GatewayStats
}

func NewHandler(
	semanticCache *cache.SemanticCache,
	llmRouter *llmrouter.LLMRouter,
	defaultProvider string,
	defaultModel string,
) *Handler {
	return &Handler{
		Cache:           semanticCache,
		LLMRouter:       llmRouter,
		DefaultProvider: defaultProvider,
		DefaultModel:    defaultModel,
		StartedAt:       time.Now().UTC(),
		Stats:           &GatewayStats{},
	}
}

func (h *Handler) Health(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()

	redisStatus := "disabled"

	if h.Cache != nil && h.Cache.Redis != nil {
		if err := h.Cache.Redis.Ping(ctx); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "unhealthy",
				"redis":  err.Error(),
			})
			return
		}

		redisStatus = "ok"
	}

	c.JSON(http.StatusOK, gin.H{
		"status":     "ok",
		"redis":      redisStatus,
		"started_at": h.StartedAt.Format(time.RFC3339),
		"uptime":     time.Since(h.StartedAt).String(),
	})
}

func (h *Handler) ChatCompletion(c *gin.Context) {
	var input chatCompletionRequest

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("invalid request body: %v", err),
		})
		return
	}

	if input.Stream {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "streaming is not implemented in milestone 7 yet",
		})
		return
	}

	if len(input.Messages) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "messages cannot be empty",
		})
		return
	}

	provider := input.Provider
	if provider == "" {
		provider = h.DefaultProvider
	}

	model := input.Model
	if model == "" {
		model = h.DefaultModel
	}

	tenantID := c.GetString(ContextTenantIDKey)
	if tenantID == "" {
		tenantID = "default"
	}

	userID := c.GetString(ContextUserIDKey)

	req := types.QueryRequest{
		TenantID:    tenantID,
		UserID:      userID,
		Provider:    provider,
		Model:       model,
		Messages:    input.Messages,
		Temperature: input.Temperature,
		Metadata:    input.Metadata,
	}

	if h.Cache != nil {
		cachedResp, decision, err := h.Cache.Lookup(c.Request.Context(), req)

		if err != nil {
			log.Printf("cache lookup failed, falling back to LLM: %v", err)
		} else if decision.Hit && cachedResp != nil {
			atomic.AddInt64(&h.Stats.CacheHits, 1)

			c.Header("X-LLM-Cache", "HIT")
			c.Header("X-LLM-Cache-Hit-Type", decision.HitType)
			c.Header("X-LLM-Cache-Similarity", fmt.Sprintf("%.4f", decision.Similarity))

			c.JSON(http.StatusOK, openAICompatibleResponse(*cachedResp))
			return
		}
	}

	atomic.AddInt64(&h.Stats.CacheMisses, 1)
	atomic.AddInt64(&h.Stats.LLMRequests, 1)

	llmResp, err := h.LLMRouter.Route(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"error": fmt.Sprintf("llm request failed: %v", err),
		})
		return
	}

	c.Header("X-LLM-Cache", "MISS")

	if h.Cache != nil {
		go func(req types.QueryRequest, resp types.LLMResponse) {
			storeCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			if err := h.Cache.Store(storeCtx, req, resp); err != nil {
				atomic.AddInt64(&h.Stats.AsyncStoreErrors, 1)
				log.Printf("async cache store failed: %v", err)
			}
		}(req, llmResp)
	}

	c.JSON(http.StatusOK, openAICompatibleResponse(llmResp))
}

func (h *Handler) CacheStats(c *gin.Context) {
	httpTotal := atomic.LoadInt64(&h.Stats.HTTPRequestsTotal)
	hits := atomic.LoadInt64(&h.Stats.CacheHits)
	misses := atomic.LoadInt64(&h.Stats.CacheMisses)
	llmRequests := atomic.LoadInt64(&h.Stats.LLMRequests)
	asyncStoreErrors := atomic.LoadInt64(&h.Stats.AsyncStoreErrors)

	totalCacheLookups := hits + misses

	hitRate := 0.0
	if totalCacheLookups > 0 {
		hitRate = float64(hits) / float64(totalCacheLookups)
	}

	c.JSON(http.StatusOK, gin.H{
		"http_requests_total": httpTotal,
		"cache_hits":          hits,
		"cache_misses":        misses,
		"cache_hit_rate":      hitRate,
		"llm_requests":        llmRequests,
		"async_store_errors":  asyncStoreErrors,
	})
}

func (h *Handler) Metrics(c *gin.Context) {
	httpTotal := atomic.LoadInt64(&h.Stats.HTTPRequestsTotal)
	hits := atomic.LoadInt64(&h.Stats.CacheHits)
	misses := atomic.LoadInt64(&h.Stats.CacheMisses)
	llmRequests := atomic.LoadInt64(&h.Stats.LLMRequests)
	asyncStoreErrors := atomic.LoadInt64(&h.Stats.AsyncStoreErrors)

	body := fmt.Sprintf(
		`llm_cache_http_requests_total %d
llm_cache_hits_total %d
llm_cache_misses_total %d
llm_cache_llm_requests_total %d
llm_cache_async_store_errors_total %d
`,
		httpTotal,
		hits,
		misses,
		llmRequests,
		asyncStoreErrors,
	)

	c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(body))
}

type chatCompletionRequest struct {
	Provider    string            `json:"provider,omitempty"`
	Model       string            `json:"model"`
	Messages    []types.Message   `json:"messages"`
	Temperature float64           `json:"temperature,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	Stream      bool              `json:"stream,omitempty"`
}

type chatCompletionResponse struct {
	ID      string                 `json:"id"`
	Object  string                 `json:"object"`
	Created int64                  `json:"created"`
	Model   string                 `json:"model"`
	Choices []chatCompletionChoice `json:"choices"`
	Usage   chatCompletionUsage    `json:"usage"`
}

type chatCompletionChoice struct {
	Index        int           `json:"index"`
	Message      types.Message `json:"message"`
	FinishReason string        `json:"finish_reason"`
}

type chatCompletionUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

func openAICompatibleResponse(resp types.LLMResponse) chatCompletionResponse {
	created := time.Now().Unix()

	return chatCompletionResponse{
		ID:      fmt.Sprintf("chatcmpl-%d", created),
		Object:  "chat.completion",
		Created: created,
		Model:   resp.Model,
		Choices: []chatCompletionChoice{
			{
				Index: 0,
				Message: types.Message{
					Role:    "assistant",
					Content: resp.Content,
				},
				FinishReason: resp.FinishReason,
			},
		},
		Usage: chatCompletionUsage{
			PromptTokens:     resp.PromptTokens,
			CompletionTokens: resp.CompletionTokens,
			TotalTokens:      resp.TotalTokens,
		},
	}
}
