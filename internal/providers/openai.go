//Implements the provider.go interface for openAI-compatible LLM calls

package providers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/k9pranav/LLM_Cache/pkg/types"
)

type OpenAIProvider struct {
	APIKey       string
	BaseURL      string
	DefaultModel string
	HTTPClient   *http.Client
}

type openAIChatRequest struct {
	Model       string          `json:"model"`
	Messages    []openAIMessage `json:"messages"`
	Temperature *float64        `json:"temperature,omitempty"`
	Stream      bool
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIChatResponse struct {
	Model string `json:"model"`

	Choices []struct {
		FinishReason string `json:"finish_reason"`
		Message      struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`

	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

type openAIStreamResponse struct {
	Model string `json:"model"`

	Choices []struct {
		Delta struct {
			Role    string `json:"role,omitempty"`
			Content string `json:"content,omitempty"`
		} `json:"delta"`

		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
}

func (p *OpenAIProvider) StreamComplete(ctx context.Context, req types.QueryRequest) (<-chan StreamChunk, error) {
	if p.APIKey == "" {
		return nil, fmt.Errorf("open ai key is empty")
	}

	model := req.Model
	if model == "" {
		model = p.DefaultModel
	}

	if len(req.Messages) == 0 {
		return nil, fmt.Errorf("request has no messages")
	}

	messages := make([]openAIMessage, 0, len(req.Messages))
	for _, msg := range req.Messages {
		messages = append(messages, openAIMessage{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	body := openAIChatRequest{
		Model:    model,
		Messages: messages,
		Stream:   true,
	}

	if req.Temperature != 0 {
		body.Temperature = &req.Temperature
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		p.BaseURL+"/chat/completions",
		bytes.NewReader(bodyBytes),
	)
	if err != nil {
		return nil, err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.APIKey)

	httpResp, err := p.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, err
	}

	if httpResp.StatusCode >= 300 {
		defer httpResp.Body.Close()

		respBytes, readErr := io.ReadAll(httpResp.Body)
		if readErr != nil {
			return nil, readErr
		}

		return nil, fmt.Errorf(
			"openai stream request failed: status=%d body=%s",
			httpResp.StatusCode,
			string(respBytes),
		)
	}

	out := make(chan StreamChunk)

	go func() {
		defer close(out)
		defer httpResp.Body.Close()

		scanner := bufio.NewScanner(httpResp.Body)
		scanner.Buffer(make([]byte, 1024), 1024*1024)

		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())

			if line == "" {
				continue
			}

			if !strings.HasPrefix(line, "data:") {
				continue
			}

			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))

			if data == "[DONE]" {
				out <- StreamChunk{
					Model: model,
					Done:  true,
				}
				return
			}

			var parsed openAIStreamResponse
			if err := json.Unmarshal([]byte(data), &parsed); err != nil {
				out <- StreamChunk{Err: err}
				return
			}

			responseModel := parsed.Model
			if responseModel == "" {
				responseModel = model
			}

			if len(parsed.Choices) == 0 {
				continue
			}

			choice := parsed.Choices[0]

			if choice.Delta.Content != "" {
				out <- StreamChunk{
					Content: choice.Delta.Content,
					Model:   responseModel,
				}
			}

			if choice.FinishReason != nil {
				out <- StreamChunk{
					FinishReason: *choice.FinishReason,
					Model:        responseModel,
					Done:         true,
				}
				return
			}
		}

		if err := scanner.Err(); err != nil {
			out <- StreamChunk{Err: err}
		}
	}()

	return out, nil

}

func NewOpenAIProvider(apiKey string, baseURL string, model string) *OpenAIProvider {
	baseURL = strings.TrimRight(baseURL, "/")

	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}

	return &OpenAIProvider{
		APIKey:       apiKey,
		BaseURL:      baseURL,
		DefaultModel: model,
		HTTPClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

func (p *OpenAIProvider) Name() string {
	return "openai"
}
func (p *OpenAIProvider) Complete(ctx context.Context, req types.QueryRequest) (types.LLMResponse, error) {

	//Safety checks
	if p.APIKey == "" {
		return types.LLMResponse{}, fmt.Errorf("openai api key is empty")
	}

	model := req.Model
	if model == "" {
		model = p.DefaultModel
	}

	if model == "" {
		return types.LLMResponse{}, fmt.Errorf("openai model is empty")
	}

	if len(req.Messages) == 0 {
		return types.LLMResponse{}, fmt.Errorf("request has no messages")
	}

	//openAIMessage is a struct defined above
	messages := make([]openAIMessage, 0, len(req.Messages))

	//msg is an element of req.Messages. Req
	for _, msg := range req.Messages {
		messages = append(messages, openAIMessage{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	body := openAIChatRequest{
		Model:    model,
		Messages: messages,
	}

	if req.Temperature != 0 {
		body.Temperature = &req.Temperature
	}

	bodyBytes, err := json.Marshal(body)

	if err != nil {
		return types.LLMResponse{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.BaseURL+"/chat/completions", bytes.NewReader(bodyBytes))

	//If I recieve error from post API to the LLM, give a empty LLMResponse with the err
	if err != nil {
		return types.LLMResponse{}, err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.APIKey)

	//Collecting the http response
	httpResp, err := p.HTTPClient.Do(httpReq)

	if err != nil {
		return types.LLMResponse{}, err
	}
	defer httpResp.Body.Close()

	respBytes, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return types.LLMResponse{}, err
	}

	if httpResp.StatusCode >= 300 {
		return types.LLMResponse{}, fmt.Errorf(
			"openai request failed: status=%d body=%s",
			httpResp.StatusCode,
			string(respBytes),
		)
	}

	var parsed openAIChatResponse

	//The json.Unmarshal 'fills' in the variable names parsed
	if err := json.Unmarshal(respBytes, &parsed); err != nil {
		return types.LLMResponse{}, err
	}

	if len(parsed.Choices) == 0 {
		return types.LLMResponse{}, fmt.Errorf("openai returned no choices")
	}

	choice := parsed.Choices[0]

	responseModel := parsed.Model
	if responseModel == "" {
		responseModel = model
	}

	return types.LLMResponse{
		Content:          strings.TrimSpace(choice.Message.Content),
		Provider:         p.Name(),
		Model:            responseModel,
		PromptTokens:     parsed.Usage.PromptTokens,
		CompletionTokens: parsed.Usage.CompletionTokens,
		TotalTokens:      parsed.Usage.TotalTokens,
		FinishReason:     choice.FinishReason,
	}, nil

}
