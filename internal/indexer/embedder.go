// Package indexer handles file walking, chunking orchestration, and embedding.
package indexer

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

// Embedder generates vector embeddings for text chunks.
type Embedder interface {
	Embed(texts []string) ([][]float32, error)
	Dimensions() int
	ModelID() string
}

// ─── NoOp (default) ───────────────────────────────────────────────────────────

// NoOpEmbedder disables vector search. BM25 still works. Zero cost, zero config.
type NoOpEmbedder struct{}

func (NoOpEmbedder) Embed(_ []string) ([][]float32, error) { return nil, nil }
func (NoOpEmbedder) Dimensions() int                       { return 0 }
func (NoOpEmbedder) ModelID() string                       { return "none" }

// ─── OpenAI ───────────────────────────────────────────────────────────────────

type OpenAIEmbedder struct {
	APIKey  string
	Model   string // default: text-embedding-3-small
	BaseURL string // default: https://api.openai.com/v1
	client  *http.Client
}

func NewOpenAIEmbedder(apiKey, model, baseURL string) *OpenAIEmbedder {
	if model == "" {
		model = "text-embedding-3-small"
	}
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	return &OpenAIEmbedder{APIKey: apiKey, Model: model, BaseURL: baseURL, client: &http.Client{}}
}

func (e *OpenAIEmbedder) Dimensions() int  { return 1536 }
func (e *OpenAIEmbedder) ModelID() string  { return e.Model }

func (e *OpenAIEmbedder) Embed(texts []string) ([][]float32, error) {
	body, _ := json.Marshal(map[string]any{"input": texts, "model": e.Model})
	req, err := http.NewRequest("POST", e.BaseURL+"/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+e.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai embed: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
		Error *struct{ Message string } `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if result.Error != nil {
		return nil, fmt.Errorf("openai api error: %s", result.Error.Message)
	}

	embeddings := make([][]float32, len(result.Data))
	for i, d := range result.Data {
		embeddings[i] = d.Embedding
	}
	return embeddings, nil
}

// ─── Ollama ───────────────────────────────────────────────────────────────────

// OllamaEmbedder calls a local Ollama instance. Free, offline, no API key.
type OllamaEmbedder struct {
	BaseURL string // default: http://localhost:11434
	Model   string // e.g. "nomic-embed-text"
	dims    int
	client  *http.Client
}

func NewOllamaEmbedder(baseURL, model string, dims int) *OllamaEmbedder {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	if dims == 0 {
		dims = 768
	}
	return &OllamaEmbedder{BaseURL: baseURL, Model: model, dims: dims, client: &http.Client{}}
}

func (e *OllamaEmbedder) Dimensions() int { return e.dims }
func (e *OllamaEmbedder) ModelID() string { return e.Model }

func (e *OllamaEmbedder) Embed(texts []string) ([][]float32, error) {
	embeddings := make([][]float32, 0, len(texts))
	for _, text := range texts {
		body, _ := json.Marshal(map[string]any{"model": e.Model, "prompt": text})
		req, err := http.NewRequest("POST", e.BaseURL+"/api/embeddings", bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := e.client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("ollama embed: %w", err)
		}
		defer resp.Body.Close()

		var result struct {
			Embedding []float32 `json:"embedding"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return nil, err
		}
		embeddings = append(embeddings, result.Embedding)
	}
	return embeddings, nil
}

// ─── Factory ──────────────────────────────────────────────────────────────────

// NewEmbedder creates an Embedder from provider name + config values.
// provider: "openai", "ollama", "none"
func NewEmbedder(provider, model, apiKey, baseURL string, dims int) (Embedder, error) {
	switch provider {
	case "openai":
		if apiKey == "" {
			return nil, fmt.Errorf("openai embedder requires an API key")
		}
		return NewOpenAIEmbedder(apiKey, model, baseURL), nil
	case "ollama":
		if model == "" {
			return nil, fmt.Errorf("ollama embedder requires a model name")
		}
		return NewOllamaEmbedder(baseURL, model, dims), nil
	case "none", "":
		return NoOpEmbedder{}, nil
	default:
		return nil, fmt.Errorf("unknown embedding provider: %s", provider)
	}
}
