package router

import (
	"context"
	"fmt"
	"strings"

	"github.com/k9pranav/LLM_Cache/internal/providers"
	"github.com/k9pranav/LLM_Cache/pkg/types"
)

type LLMRouter struct {
	providers       map[string]providers.Provider
	defaultProvider string
}

func NewLLMRouter(providerMap map[string]providers.Provider, defaultProvider string) *LLMRouter {

	normalized := make(map[string]providers.Provider)

	for name, provider := range providerMap {
		normalizedName := strings.ToLower(strings.TrimSpace(name))
		normalized[normalizedName] = provider
	}

	return &LLMRouter{
		providers:       normalized,
		defaultProvider: strings.ToLower(strings.TrimSpace(defaultProvider)),
	}
}

func (r *LLMRouter) Route(ctx context.Context, req types.QueryRequest) (types.LLMResponse, error) {
	providerName := strings.ToLower(strings.TrimSpace(req.Provider))

	if providerName == "" {
		providerName = r.defaultProvider
	}

	if providerName == "" {
		return types.LLMResponse{}, fmt.Errorf("no provider specified and no default provider configured")
	}

	provider, ok := r.providers[providerName]
	if !ok {
		return types.LLMResponse{}, fmt.Errorf("provider not found: %s", providerName)
	}

	resp, err := provider.Complete(ctx, req)
	if err != nil {
		return types.LLMResponse{}, err
	}

	if resp.Provider == "" {
		resp.Provider = provider.Name()
	}

	if resp.Model == "" {
		resp.Model = req.Model
	}

	return resp, nil

}

func (r *LLMRouter) RouteStream(
	ctx context.Context,
	req types.QueryRequest,
) (<-chan providers.StreamChunk, error) {
	providerName := strings.ToLower(strings.TrimSpace(req.Provider))

	if providerName == "" {
		providerName = r.defaultProvider
	}

	if providerName == "" {
		return nil, fmt.Errorf("no provider specified and no default provider configured")
	}

	provider, ok := r.providers[providerName]
	if !ok {
		return nil, fmt.Errorf("provider not found: %s", providerName)
	}

	streamingProvider, ok := provider.(providers.StreamingProvider)
	if !ok {
		return nil, fmt.Errorf("provider does not support streaming: %s", providerName)
	}

	return streamingProvider.StreamComplete(ctx, req)
}
