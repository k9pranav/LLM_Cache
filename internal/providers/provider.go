package providers

import (
	"context"

	"github.com/k9pranav/LLM_Cache/pkg/types"
)

// Normal non-streaming provider
type Provider interface {
	Complete(ctx context.Context, req types.QueryRequest) (types.LLMResponse, error)
	Name() string
}

// provider that supports streaming
type StreamingProvider interface {
	Provider
	StreamComplete(ctx context.Context, req types.QueryRequest) (<-chan StreamChunk, error)
}

// One decoded content delta from SSE
type StreamChunk struct {
	Content      string
	FinishReason string
	Model        string
	Done         bool
	Err          error
}
