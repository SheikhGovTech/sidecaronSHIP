package cli

import (
	"log/slog"
	"os"

	"github.com/sausheong/sidecar/internal/config"
	"github.com/sausheong/sidecar/internal/memory"
)

// buildEmbeddingProvider constructs an EmbeddingProvider from the embedding config.
// Returns nil if embedding is not configured — memory is then disabled silently.
func buildEmbeddingProvider(cfg *config.Config) memory.EmbeddingProvider {
	baseURL := cfg.Embedding.BaseURL
	apiKeyEnv := cfg.Embedding.APIKeyEnv
	if apiKeyEnv == "" {
		apiKeyEnv = "OPENAI_API_KEY"
	}
	apiKey := os.Getenv(apiKeyEnv)
	switch cfg.Embedding.Provider {
	case "openai":
		if apiKey == "" {
			slog.Warn("embedding API key not set, memory disabled", "provider", "openai", "env", apiKeyEnv)
			return nil
		}
		if baseURL == "" {
			return memory.NewOpenAIWithBaseURLAndDimensions(apiKey, cfg.Embedding.Model, "https://api.openai.com", cfg.Embedding.Dimensions)
		}
		return memory.NewOpenAIWithBaseURLAndDimensions(apiKey, cfg.Embedding.Model, baseURL, cfg.Embedding.Dimensions)
	case "cohere":
		if cfg.Embedding.APIKeyEnv == "" {
			apiKeyEnv = "COHERE_API_KEY"
			apiKey = os.Getenv(apiKeyEnv)
		}
		if apiKey == "" {
			slog.Warn("embedding API key not set, memory disabled", "provider", "cohere", "env", apiKeyEnv)
			return nil
		}
		if baseURL == "" {
			return memory.NewCohere(apiKey, cfg.Embedding.Model)
		}
		return memory.NewCohereWithBaseURLAndDimensions(apiKey, cfg.Embedding.Model, baseURL, cfg.Embedding.Dimensions)
	case "voyage":
		if cfg.Embedding.APIKeyEnv == "" {
			apiKeyEnv = "VOYAGE_API_KEY"
			apiKey = os.Getenv(apiKeyEnv)
		}
		if apiKey == "" {
			slog.Warn("embedding API key not set, memory disabled", "provider", "voyage", "env", apiKeyEnv)
			return nil
		}
		if baseURL != "" {
			return memory.NewVoyageWithBaseURL(apiKey, cfg.Embedding.Model, baseURL)
		}
		return memory.NewVoyage(apiKey, cfg.Embedding.Model)
	default:
		return nil // no embedding configured — memory disabled
	}
}
