package rag

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

// GetEmbedding converts text to a vector using Google Gemini's free embedding API
func GetEmbedding(text string) ([]float64, error) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY not set — get a free key at aistudio.google.com")
	}

	url := fmt.Sprintf(
		"https://generativelanguage.googleapis.com/v1beta/models/gemini-embedding-001:embedContent?key=%s",
		apiKey,
	)

	body, _ := json.Marshal(map[string]interface{}{
		"model": "models/gemini-embedding-001",
		"content": map[string]interface{}{
			"parts": []map[string]string{
				{"text": text},
			},
		},
		"taskType": "RETRIEVAL_DOCUMENT",
	})

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("embedding request failed: %w", err)
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)

	var result struct {
		Embedding struct {
			Values []float64 `json:"values"`
		} `json:"embedding"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse embedding response: %w", err)
	}

	if result.Error != nil {
		return nil, fmt.Errorf("Gemini embedding error: %s", result.Error.Message)
	}

	if len(result.Embedding.Values) == 0 {
		return nil, fmt.Errorf("empty embedding returned — raw response: %s", string(data))
	}

	return result.Embedding.Values, nil
}

// EmbedChunks embeds all chunks and returns their vectors
func EmbedChunks(chunks []Chunk) ([][]float64, error) {
	embeddings := make([][]float64, len(chunks))
	for i, chunk := range chunks {
		emb, err := GetEmbedding(chunk.Text)
		if err != nil {
			return nil, fmt.Errorf("failed to embed chunk %d: %w", i, err)
		}
		embeddings[i] = emb
	}
	return embeddings, nil
}
