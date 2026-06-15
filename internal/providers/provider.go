package providers

import (
	"context"

	"github.com/k9pranav/LLM_Cache/pkg/types"
)

type Provider interface {
	Complete(ctx context.Context, req types.QueryRequest) (types.LLMResponse, error)
	Name() string
}
