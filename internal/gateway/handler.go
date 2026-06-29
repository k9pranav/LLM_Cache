package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/gin-gonic/gin"
	"github.com/k9pranav/LLM_Cache/internal/cache"
	llmrouter "github.com/k9pranav/LLM_Cache/internal/router"
	"github.com/k9pranav/LLM_Cache/pkg/types"
)

var streamBufPool = sync.Pool{
	New: func() any {
		b := make([]byte, 0, 4096)
		return &b
	},
}

type GatewayStats struct {
	HTTPRequestsTotal int64
	CacheHits         int64
	CacheMisses       int64
	LLMRequests       int64
	StreamingRequests int64
	AsyncStoreErrors  int64
	AsyncStoreSkipped int64
}

type Handler struct {
	Cache               *cache.SemanticCache
	LLMRouter           *llmrouter.LLMRouter
	DefaultProvider     string
	DefaultModel        string
	StartedAt           time.Time
	Stats               *GatewayStats
	AsyncStoreSemaphore chan struct{}
}

type chatCompletionStreamResponse struct {
	ID      string                       `json:"id"`
	Object  string                       `json:"object"`
	Created int64                        `json:"created"`
	Model   string                       `json:"model"`
	Choices []chatCompletionStreamChoice `json:"choices"`
}

type chatCompletionStreamChoice struct {
	Index        int                       `json:"index"`
	Delta        chatCompletionStreamDelta `json:"delta"`
	FinishReason *string                   `json:"finish_reason"`
}

type chatCompletionStreamDelta struct {
	Role    string `json:"role,omitempty"`
	Content string `json:"content,omitempty"`
}

type askRequest struct {
	Prompt      string            `json:"prompt"`
	System      string            `json:"system,omitempty"`
	Provider    string            `json:"provider,omitempty"`
	Model       string            `json:"model,omitempty"`
	Temperature float64           `json:"temperature,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	Stream      bool              `json:"stream,omitempty"`
}

func NewHandler(
	semanticCache *cache.SemanticCache,
	llmRouter *llmrouter.LLMRouter,
	defaultProvider string,
	defaultModel string,
) *Handler {
	return &Handler{
		Cache:               semanticCache,
		LLMRouter:           llmRouter,
		DefaultProvider:     defaultProvider,
		DefaultModel:        defaultModel,
		StartedAt:           time.Now().UTC(),
		Stats:               &GatewayStats{},
		AsyncStoreSemaphore: make(chan struct{}, 100),
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

			if input.Stream {
				h.writeCachedStream(c, *cachedResp)
				return
			}

			c.JSON(http.StatusOK, openAICompatibleResponse(*cachedResp))
			return
		}
	}

	atomic.AddInt64(&h.Stats.CacheMisses, 1)
	atomic.AddInt64(&h.Stats.LLMRequests, 1)

	c.Header("X-LLM-Cache", "MISS")

	if input.Stream {
		h.ChatCompletionStream(c, req)
		return
	}

	llmResp, err := h.LLMRouter.Route(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"error": fmt.Sprintf("llm request failed: %v", err),
		})
		return
	}

	h.storeAsync(req, llmResp)

	c.JSON(http.StatusOK, openAICompatibleResponse(llmResp))
}

func (h *Handler) ChatCompletionStream(c *gin.Context, req types.QueryRequest) {
	atomic.AddInt64(&h.Stats.StreamingRequests, 1)

	stream, err := h.LLMRouter.RouteStream(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"error": fmt.Sprintf("llm stream failed: %v", err),
		})
		return
	}

	setupSSEHeaders(c)

	streamID := fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano())
	created := time.Now().Unix()
	model := req.Model
	finishReason := "stop"

	var fullResponse strings.Builder

	for chunk := range stream {
		if chunk.Err != nil {
			log.Printf("llm stream chunk error: %v", chunk.Err)
			_ = writeSSEError(c, chunk.Err)
			return
		}

		if chunk.Model != "" {
			model = chunk.Model
		}

		if chunk.Content != "" {
			fullResponse.WriteString(chunk.Content)

			if err := writeSSEContentChunk(c, streamID, created, model, chunk.Content); err != nil {
				log.Printf("failed writing SSE content chunk: %v", err)
				return
			}
		}

		if chunk.FinishReason != "" {
			finishReason = chunk.FinishReason
		}

		if chunk.Done {
			break
		}
	}

	finalResp := types.LLMResponse{
		Content:      fullResponse.String(),
		Provider:     req.Provider,
		Model:        model,
		FinishReason: finishReason,
	}

	if err := writeSSEFinalChunk(c, streamID, created, model, finishReason); err != nil {
		log.Printf("failed writing SSE final chunk: %v", err)
		return
	}

	if err := writeSSEDone(c); err != nil {
		log.Printf("failed writing SSE done chunk: %v", err)
		return
	}

	h.storeAsync(req, finalResp)
}

func (h *Handler) storeAsync(req types.QueryRequest, resp types.LLMResponse) {
	if h.Cache == nil {
		return
	}

	select {
	case h.AsyncStoreSemaphore <- struct{}{}:
	default:
		atomic.AddInt64(&h.Stats.AsyncStoreSkipped, 1)
		log.Printf("async cache store skipped: too many background stores")
		return
	}

	go func() {
		defer func() {
			<-h.AsyncStoreSemaphore
		}()

		storeCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := h.Cache.Store(storeCtx, req, resp); err != nil {
			atomic.AddInt64(&h.Stats.AsyncStoreErrors, 1)
			log.Printf("async cache store failed: %v", err)
		}
	}()
}

func setupSSEHeaders(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)
}

func writeSSEContentChunk(
	c *gin.Context,
	id string,
	created int64,
	model string,
	content string,
) error {
	payload := chatCompletionStreamResponse{
		ID:      id,
		Object:  "chat.completion.chunk",
		Created: created,
		Model:   model,
		Choices: []chatCompletionStreamChoice{
			{
				Index: 0,
				Delta: chatCompletionStreamDelta{
					Content: content,
				},
				FinishReason: nil,
			},
		},
	}

	return writeSSEJSON(c, payload)
}

func writeSSEFinalChunk(
	c *gin.Context,
	id string,
	created int64,
	model string,
	finishReason string,
) error {
	payload := chatCompletionStreamResponse{
		ID:      id,
		Object:  "chat.completion.chunk",
		Created: created,
		Model:   model,
		Choices: []chatCompletionStreamChoice{
			{
				Index:        0,
				Delta:        chatCompletionStreamDelta{},
				FinishReason: &finishReason,
			},
		},
	}

	return writeSSEJSON(c, payload)
}

func writeSSEJSON(c *gin.Context, payload any) error {
	jsonBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	bufPtr := streamBufPool.Get().(*[]byte)

	buf := (*bufPtr)[:0]
	buf = append(buf, "data: "...)
	buf = append(buf, jsonBytes...)
	buf = append(buf, "\n\n"...)

	_, err = c.Writer.Write(buf)
	c.Writer.Flush()

	if cap(buf) > 64*1024 {
		newBuf := make([]byte, 0, 4096)
		*bufPtr = newBuf
	} else {
		*bufPtr = buf[:0]
	}

	streamBufPool.Put(bufPtr)

	return err
}

func writeSSEDone(c *gin.Context) error {
	bufPtr := streamBufPool.Get().(*[]byte)

	buf := (*bufPtr)[:0]
	buf = append(buf, "data: [DONE]\n\n"...)

	_, err := c.Writer.Write(buf)
	c.Writer.Flush()

	*bufPtr = buf[:0]
	streamBufPool.Put(bufPtr)

	return err
}

func writeSSEError(c *gin.Context, streamErr error) error {
	payload := map[string]string{
		"error": streamErr.Error(),
	}

	jsonBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	bufPtr := streamBufPool.Get().(*[]byte)

	buf := (*bufPtr)[:0]
	buf = append(buf, "event: error\n"...)
	buf = append(buf, "data: "...)
	buf = append(buf, jsonBytes...)
	buf = append(buf, "\n\n"...)

	_, err = c.Writer.Write(buf)
	c.Writer.Flush()

	*bufPtr = buf[:0]
	streamBufPool.Put(bufPtr)

	return err
}

func (h *Handler) writeCachedStream(c *gin.Context, resp types.LLMResponse) {
	setupSSEHeaders(c)

	streamID := fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano())
	created := time.Now().Unix()

	err := streamTextInChunks(
		resp.Content,
		25*time.Millisecond,
		func(part string) error {
			return writeSSEContentChunk(c, streamID, created, resp.Model, part)
		},
	)

	if err != nil {
		log.Printf("failed writing cached SSE content chunks: %v", err)
		return
	}

	finishReason := resp.FinishReason
	if finishReason == "" {
		finishReason = "stop"
	}

	if err := writeSSEFinalChunk(c, streamID, created, resp.Model, finishReason); err != nil {
		log.Printf("failed writing cached SSE final chunk: %v", err)
		return
	}

	if err := writeSSEDone(c); err != nil {
		log.Printf("failed writing cached SSE done chunk: %v", err)
		return
	}
}

func (h *Handler) CacheStats(c *gin.Context) {
	httpTotal := atomic.LoadInt64(&h.Stats.HTTPRequestsTotal)
	hits := atomic.LoadInt64(&h.Stats.CacheHits)
	misses := atomic.LoadInt64(&h.Stats.CacheMisses)
	llmRequests := atomic.LoadInt64(&h.Stats.LLMRequests)
	streamingRequests := atomic.LoadInt64(&h.Stats.StreamingRequests)
	asyncStoreErrors := atomic.LoadInt64(&h.Stats.AsyncStoreErrors)
	asyncStoreSkipped := atomic.LoadInt64(&h.Stats.AsyncStoreSkipped)

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
		"streaming_requests":  streamingRequests,
		"async_store_errors":  asyncStoreErrors,
		"async_store_skipped": asyncStoreSkipped,
	})
}

func (h *Handler) Metrics(c *gin.Context) {
	httpTotal := atomic.LoadInt64(&h.Stats.HTTPRequestsTotal)
	hits := atomic.LoadInt64(&h.Stats.CacheHits)
	misses := atomic.LoadInt64(&h.Stats.CacheMisses)
	llmRequests := atomic.LoadInt64(&h.Stats.LLMRequests)
	streamingRequests := atomic.LoadInt64(&h.Stats.StreamingRequests)
	asyncStoreErrors := atomic.LoadInt64(&h.Stats.AsyncStoreErrors)
	asyncStoreSkipped := atomic.LoadInt64(&h.Stats.AsyncStoreSkipped)

	body := fmt.Sprintf(
		`llm_cache_http_requests_total %d
llm_cache_hits_total %d
llm_cache_misses_total %d
llm_cache_llm_requests_total %d
llm_cache_streaming_requests_total %d
llm_cache_async_store_errors_total %d
llm_cache_async_store_skipped_total %d
`,
		httpTotal,
		hits,
		misses,
		llmRequests,
		streamingRequests,
		asyncStoreErrors,
		asyncStoreSkipped,
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
func (h *Handler) buildAskQueryRequest(c *gin.Context, input askRequest) types.QueryRequest {
	provider := input.Provider
	if provider == "" {
		provider = h.DefaultProvider
	}

	model := input.Model
	if model == "" {
		model = h.DefaultModel
	}

	systemPrompt := input.System
	if systemPrompt == "" {
		systemPrompt = "You are a helpful technical assistant."
	}

	tenantID := c.GetString(ContextTenantIDKey)
	if tenantID == "" {
		tenantID = "default"
	}

	userID := c.GetString(ContextUserIDKey)

	return types.QueryRequest{
		TenantID: tenantID,
		UserID:   userID,
		Provider: provider,
		Model:    model,
		Messages: []types.Message{
			{
				Role:    "system",
				Content: systemPrompt,
			},
			{
				Role:    "user",
				Content: input.Prompt,
			},
		},
		Temperature: input.Temperature,
		Metadata:    input.Metadata,
	}
}
func (h *Handler) Ask(c *gin.Context) {
	var input askRequest

	if err := c.ShouldBindJSON(&input); err != nil {
		c.String(http.StatusBadRequest, "invalid request body: %v", err)
		return
	}

	if strings.TrimSpace(input.Prompt) == "" {
		c.String(http.StatusBadRequest, "prompt cannot be empty")
		return
	}

	if input.Stream {
		req := h.buildAskQueryRequest(c, input)
		h.askStreamInternal(c, req)
		return
	}

	req := h.buildAskQueryRequest(c, input)

	if h.Cache != nil {
		cachedResp, decision, err := h.Cache.Lookup(c.Request.Context(), req)

		if err != nil {
			log.Printf("cache lookup failed, falling back to LLM: %v", err)
		} else if decision.Hit && cachedResp != nil {
			atomic.AddInt64(&h.Stats.CacheHits, 1)

			c.Header("X-LLM-Cache", "HIT")
			c.Header("X-LLM-Cache-Hit-Type", decision.HitType)
			c.Header("X-LLM-Cache-Similarity", fmt.Sprintf("%.4f", decision.Similarity))

			c.String(http.StatusOK, cachedResp.Content)
			return
		}
	}

	atomic.AddInt64(&h.Stats.CacheMisses, 1)
	atomic.AddInt64(&h.Stats.LLMRequests, 1)

	llmResp, err := h.LLMRouter.Route(c.Request.Context(), req)
	if err != nil {
		c.String(http.StatusBadGateway, "llm request failed: %v", err)
		return
	}

	c.Header("X-LLM-Cache", "MISS")

	h.storeAsync(req, llmResp)

	c.String(http.StatusOK, llmResp.Content)
}

func (h *Handler) AskStream(c *gin.Context) {
	var input askRequest

	if err := c.ShouldBindJSON(&input); err != nil {
		c.String(http.StatusBadRequest, "invalid request body: %v", err)
		return
	}

	if strings.TrimSpace(input.Prompt) == "" {
		c.String(http.StatusBadRequest, "prompt cannot be empty")
		return
	}

	req := h.buildAskQueryRequest(c, input)
	h.askStreamInternal(c, req)
}

func (h *Handler) askStreamInternal(c *gin.Context, req types.QueryRequest) {
	if h.Cache != nil {
		cachedResp, decision, err := h.Cache.Lookup(c.Request.Context(), req)

		if err != nil {
			log.Printf("cache lookup failed, falling back to LLM: %v", err)
		} else if decision.Hit && cachedResp != nil {
			atomic.AddInt64(&h.Stats.CacheHits, 1)

			c.Header("X-LLM-Cache", "HIT")
			c.Header("X-LLM-Cache-Hit-Type", decision.HitType)
			c.Header("X-LLM-Cache-Similarity", fmt.Sprintf("%.4f", decision.Similarity))

			setupPlainTextStreamHeaders(c)

			err := streamTextInChunks(
				cachedResp.Content,
				25*time.Millisecond,
				func(part string) error {
					return writePlainTextChunk(c, part)
				},
			)

			if err != nil {
				log.Printf("failed writing cached plain text stream: %v", err)
			}

			return
		}
	}

	atomic.AddInt64(&h.Stats.CacheMisses, 1)
	atomic.AddInt64(&h.Stats.LLMRequests, 1)
	atomic.AddInt64(&h.Stats.StreamingRequests, 1)

	c.Header("X-LLM-Cache", "MISS")

	stream, err := h.LLMRouter.RouteStream(c.Request.Context(), req)
	if err != nil {
		c.String(http.StatusBadGateway, "llm stream failed: %v", err)
		return
	}

	setupPlainTextStreamHeaders(c)

	var fullResponse strings.Builder

	model := req.Model
	finishReason := "stop"

	for chunk := range stream {
		if chunk.Err != nil {
			log.Printf("llm stream chunk error: %v", chunk.Err)
			return
		}

		if chunk.Model != "" {
			model = chunk.Model
		}

		if chunk.Content != "" {
			fullResponse.WriteString(chunk.Content)

			if err := writePlainTextChunk(c, chunk.Content); err != nil {
				log.Printf("failed writing plain text stream chunk: %v", err)
				return
			}

			time.Sleep(15 * time.Millisecond)
		}

		if chunk.FinishReason != "" {
			finishReason = chunk.FinishReason
		}

		if chunk.Done {
			break
		}
	}

	finalResp := types.LLMResponse{
		Content:      fullResponse.String(),
		Provider:     req.Provider,
		Model:        model,
		FinishReason: finishReason,
	}

	h.storeAsync(req, finalResp)
}
func setupPlainTextStreamHeaders(c *gin.Context) {
	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)
}

func writePlainTextChunk(c *gin.Context, text string) error {
	bufPtr := streamBufPool.Get().(*[]byte)

	buf := (*bufPtr)[:0]
	buf = append(buf, text...)

	_, err := c.Writer.Write(buf)
	c.Writer.Flush()

	if cap(buf) > 64*1024 {
		newBuf := make([]byte, 0, 4096)
		*bufPtr = newBuf
	} else {
		*bufPtr = buf[:0]
	}

	streamBufPool.Put(bufPtr)

	return err
}
func streamTextInChunks(text string, delay time.Duration, writeFn func(string) error) error {
	var chunk strings.Builder

	for _, r := range text {
		chunk.WriteRune(r)

		if shouldFlushTextChunk(r, chunk.Len()) {
			if err := writeFn(chunk.String()); err != nil {
				return err
			}

			chunk.Reset()

			if delay > 0 {
				time.Sleep(delay)
			}
		}
	}

	if chunk.Len() > 0 {
		if err := writeFn(chunk.String()); err != nil {
			return err
		}
	}

	return nil
}

func shouldFlushTextChunk(r rune, currentLen int) bool {
	if unicode.IsSpace(r) {
		return true
	}

	if r == '.' || r == ',' || r == ';' || r == ':' || r == '!' || r == '?' || r == '\n' {
		return true
	}

	return currentLen >= 24
}
