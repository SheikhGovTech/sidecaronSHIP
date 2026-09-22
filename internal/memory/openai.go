package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const openAIBase = "https://api.openai.com"
const openAIDefaultModel = "text-embedding-3-small"

// OpenAIProvider embeds text using OpenAI's embedding API.
type OpenAIProvider struct {
	apiKey  string
	model   string
	baseURL string
	dims    int
	client  *http.Client
}

// NewOpenAI creates a provider using the OpenAI API.
func NewOpenAI(apiKey, model string) *OpenAIProvider {
	return NewOpenAIWithBaseURL(apiKey, model, openAIBase)
}

// NewOpenAIWithBaseURL creates a provider with a custom base URL (used in tests).
func NewOpenAIWithBaseURL(apiKey, model, baseURL string) *OpenAIProvider {
	return NewOpenAIWithBaseURLAndDimensions(apiKey, model, baseURL, 1024)
}

// NewOpenAIWithBaseURLAndDimensions creates an OpenAI-compatible provider with
// an explicit output dimension.
func NewOpenAIWithBaseURLAndDimensions(apiKey, model, baseURL string, dimensions int) *OpenAIProvider {
	if model == "" {
		model = openAIDefaultModel
	}
	if dimensions <= 0 {
		dimensions = 1024
	}
	return &OpenAIProvider{
		apiKey:  apiKey,
		model:   model,
		baseURL: baseURL,
		dims:    dimensions,
		client:  &http.Client{Timeout: 30 * time.Second},
	}
}

// Dims returns 1024 (reduced via dimensions param to match schema).
func (p *OpenAIProvider) Dims() int { return p.dims }

// Embed calls the OpenAI embeddings API. inputType is ignored (OpenAI has no query/document distinction).
func (p *OpenAIProvider) Embed(ctx context.Context, texts []string, inputType string) ([][]float32, error) {
	payload := map[string]any{
		"input":      texts,
		"model":      p.model,
		"dimensions": p.dims,
	}
	if inputType != "" {
		payload["input_type"] = inputType
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshalling openai request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/v1/embeddings", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai embed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openai embed status %d", resp.StatusCode)
	}

	var result struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
			Index     int       `json:"index"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decoding openai response: %w", err)
	}

	embeddings := make([][]float32, len(texts))
	for _, d := range result.Data {
		if d.Index < len(embeddings) {
			embeddings[d.Index] = d.Embedding
		}
	}
	for i, embedding := range embeddings {
		if len(embedding) != p.dims {
			return nil, fmt.Errorf("embedding %d has dimension %d, expected %d", i, len(embedding), p.dims)
		}
	}
	return embeddings, nil
}
