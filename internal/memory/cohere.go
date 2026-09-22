package memory

import "context"

const cohereBase = "https://api.cohere.com"

// CohereProvider uses Cohere's OpenAI-compatible embeddings response shape.
// It keeps provider selection explicit while allowing a compatible gateway URL.
type CohereProvider struct{ *OpenAIProvider }

func NewCohere(apiKey, model string) *CohereProvider {
	return NewCohereWithBaseURL(apiKey, model, cohereBase)
}

func NewCohereWithBaseURL(apiKey, model, baseURL string) *CohereProvider {
	if model == "" {
		model = "cohere.embed-english-v3"
	}
	return &CohereProvider{NewOpenAIWithBaseURL(apiKey, model, baseURL)}
}

func NewCohereWithBaseURLAndDimensions(apiKey, model, baseURL string, dimensions int) *CohereProvider {
	if model == "" {
		model = "cohere.embed-english-v3"
	}
	return &CohereProvider{NewOpenAIWithBaseURLAndDimensions(apiKey, model, baseURL, dimensions)}
}

// Embed maps Sidecar's internal input types to Cohere/PAi's names.
func (p *CohereProvider) Embed(ctx context.Context, texts []string, inputType string) ([][]float32, error) {
	switch inputType {
	case "query":
		inputType = "search_query"
	case "document":
		inputType = "search_document"
	}
	return p.OpenAIProvider.Embed(ctx, texts, inputType)
}
