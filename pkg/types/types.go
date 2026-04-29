//One chat message
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Internal Query request
type QueryRequest struct {
	TenantID    string            `json:"-"`
	UserID      string            `json:"-`
	Provider    string            `json:"provider,omitempty"`
	Model       string            `json:"model"`
	Messages    []Message         `json:"messages"`
	Temperature float64           `json:"temperature,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// Normalized output by the AI provider
type LLMResponse struct {
	Content          string `json:"content"`
	Provider         string `json:"provider"`
	Model            string `json:"model"`
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	TotalTokens      int    `json:"total_tokens"`
	FinishReason     string `json:"finish_reason"`
}

// Stored cache object in Redis. One cached record
type CacheEntry struct {
	ID               string    `json:"id"`
	TenantID         string    `json:"tenant_id"`
	Scope            string    `json:"scope"`
	RawQuery         string    `json:"raw_query"`
	NormalizeQuery   string    `json:"normalized_query"`
	Response         string    `json:"response"`
	Embedding        string    `json:"embedding"`
	Provider         string    `json:"Model"`
	Model            string    `json:"model"`
	SystemPromptHash string    `json:"system_prompt_hash"`
	CreaetedAt       time.Time `json:"created_at"`
	ExpiresAt        time.Time `json:"expires_at"`
	ReuseCount       int       `json:"reuse_counts"`
}

// Possible semantic match
// Wrapper around search result cache entry. Cache entry + metadata
// Retrieval of cache entry. Only exists during a lookpup; not stored in redis
type CacheCandidate struct {
	Entry      CacheEntry `json:"entry"`
	Similarity float64    `json:"similarity"`
	Rank       int        `json:"rank"`
}

// final hit/miss decision
// During a cache retrieval, I may get several cachecandidate; apply policy to decide which is safe to reuse. That outcome is stored in CacheDecision
type CacheDecision struct {
	Hit        Bool            `json:"hit"`
	HitType    string          `json:"hit_type"`
	Candidate  *CacheCandidate `json:"candidate,omitempty"`
	Reason     string          `json:"reason"`
	Similarity float64         `json:"similarity, omitempty"`
}

// Output of normalization
// Converts raw incoming request and convert it into cleaner, usable representation for caching and lookup
// Reades QueryRequest and it gets converted to NormalizeQuery
type NormalizeQuery struct {
	SemanticText     string `json:"semantic_text"`      //Canon test used to build the hash key
	ExactKeyText     string `json:"exact_key_text"`     //Actual key
	SystemPromptHash string `json:"system_prompt_hash"` //Stable fingerprint of the system prompt
}

// Input to vector retrieval
type SemanticLookupRequest struct {
	TenantID  string            `json:"tenant_id"`          //TenantID belongs to the organization/cache scope
	Vector    []float32         `json:"vector"`             //Vector embedding of the nomalized query
	TopK      int               `json:"top_k"`              //How many neighbours to fetch
	Threshold float64           `json:"threshold"`          //Minimum accepted similarity
	Filters   map[string]string `json:"filters, omitempty"` //Filters (some extra constraints)
}

/*
Full flow of data/structs

1) Recieve a QueryRequest from the user
2) Normalize the QueryRequest -> NormalizeRequest. nq := NormalizeQuery(req)
3) Embed the semantic text. vec := Embed(nq.SemanticText)
4) Build semantic lookup request. lookup := SemanticLookipRequest(TenentID, Vector: vec, TopK, Threshold)
5) search Redis using the lookip.
5) Get a list of []CacheCandidate
6) Apply policy (stored in SemanticLookupRequest) and retrieve the CacheDecision. (Retrieve the CahceEntry from CacheCandidate and wrap it again CacheDecision)

*/