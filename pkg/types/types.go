package types

import "time"

// One chat message from the OpenAI-style request.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Incoming internal request.
type QueryRequest struct {
	TenantID    string            `json:"-"`
	UserID      string            `json:"-"`
	Provider    string            `json:"provider,omitempty"`
	Model       string            `json:"model"`
	Messages    []Message         `json:"messages"`
	Temperature float64           `json:"temperature,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// Normalized output from the LLM provider.
type LLMResponse struct {
	Content          string `json:"content"`
	Provider         string `json:"provider"`
	Model            string `json:"model"`
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	TotalTokens      int    `json:"total_tokens"`
	FinishReason     string `json:"finish_reason"`
}

// One cached record stored in Redis.
type CacheEntry struct {
	ID               string    `json:"id"`
	TenantID         string    `json:"tenant_id"`
	Scope            string    `json:"scope"`
	RawQuery         string    `json:"raw_query"`
	NormalizedQuery  string    `json:"normalized_query"`
	Response         string    `json:"response"`
	Provider         string    `json:"provider"`
	Model            string    `json:"model"`
	SystemPromptHash string    `json:"system_prompt_hash"`
	CreatedAt        time.Time `json:"created_at"`
	ExpiresAt        time.Time `json:"expires_at"`
	ReuseCount       int       `json:"reuse_count"`
}

// Redis semantic search result.
type CacheCandidate struct {
	Entry      CacheEntry `json:"entry"`
	Similarity float64    `json:"similarity"`
	Rank       int        `json:"rank"`
}

// Final cache hit/miss decision.
type CacheDecision struct {
	Hit        bool            `json:"hit"`
	HitType    string          `json:"hit_type"`
	Candidate  *CacheCandidate `json:"candidate,omitempty"`
	Reason     string          `json:"reason"`
	Similarity float64         `json:"similarity,omitempty"`
}

// Output of the conversation/context builder.
type NormalizedQuery struct {
	SemanticText     string `json:"semantic_text"`
	ExactKeyText     string `json:"exact_key_text"`
	SystemPromptHash string `json:"system_prompt_hash"`
}

// Input to Redis vector search.
type SemanticLookupRequest struct {
	TenantID  string            `json:"tenant_id"`
	Vector    []float32         `json:"vector"`
	TopK      int               `json:"top_k"`
	Threshold float64           `json:"threshold"`
	Filters   map[string]string `json:"filters,omitempty"`
}
