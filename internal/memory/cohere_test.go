package memory_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sausheong/sidecar/internal/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCohereProvider_EmbedSendsInputType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/embeddings", r.URL.Path)
		assert.Equal(t, "Bearer key", r.Header.Get("Authorization"))
		var body map[string]any
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, "cohere.embed-english-v3", body["model"])
		assert.Equal(t, "search_query", body["input_type"])
		w.Header().Set("Content-Type", "application/json")
		vector := make([]float32, 1024)
		vector[0], vector[1] = 0.1, 0.2
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{map[string]any{"embedding": vector, "index": 0}}})
	}))
	defer server.Close()

	p := memory.NewCohereWithBaseURL("key", "", server.URL)
	got, err := p.Embed(context.Background(), []string{"text"}, "search_query")
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, float32(0.1), got[0][0])
	assert.Equal(t, float32(0.2), got[0][1])
	assert.Len(t, got[0], 1024)
}
