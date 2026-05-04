package embedder

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Holds base URL, model name, http client
type Embedder struct {
	BaseURL string
	Model   string
	Client  *http.Client //http client; uses embedRequest
}

func CreateEmbedder(BaseURL string, model string) *Embedder {
	return &Embedder{
		BaseURL: BaseURL,
		Model:   model,
		Client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Function that returns MxbaiEmbedder with the ollama specs
func NewMxbaiEmbedder() *Embedder {
	return &Embedder{
		BaseURL: "http://localhost:11434",
		Model:   "mxbai-embed-large",
		Client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

type embedRequest struct {
	Model string `json:"model"`
	Input string `json:"input"`
}

type embedResponse struct {
	Model      string      `json:"model"`
	Embeddings [][]float64 `json:"embeddings"`
}

func (e *Embedder) Embed(text string) ([]float64, error) {
	var reqBody embedRequest = embedRequest{
		Model: e.Model,
		Input: text,
	}

	//Converting the struct into a json for curl
	bodyBytes, err := json.Marshal(reqBody)

	if err != nil {
		return nil, err
	}

	url := e.BaseURL + "/api/embed"

	resp, err := e.Client.Post(url, "application/json", bytes.NewReader(bodyBytes))

	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("embed api failed with status %s", resp.Status)
	}

	var result embedResponse

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	if len(result.Embeddings) == 0 {
		return nil, fmt.Errorf("ollama returned no embeddings")
	}

	//Note, the ollama result is [[]]
	return result.Embeddings[0], nil

}
