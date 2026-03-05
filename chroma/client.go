package chroma

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

const (
	chromaURL      = "http://localhost:8000"
	chromaTenant   = "default_tenant"
	chromaDatabase = "default_database"
)

// Client wraps ChromaDB HTTP API (v2)
type Client struct {
	baseURL    string
	tenant     string
	database   string
	collection string
	httpClient *http.Client
}

// NewClient creates a new ChromaDB v2 client
func NewClient(collection string) *Client {
	return &Client{
		baseURL:    chromaURL,
		tenant:     chromaTenant,
		database:   chromaDatabase,
		collection: collection,
		httpClient: &http.Client{},
	}
}

// base path for all v2 API calls
func (c *Client) base() string {
	return fmt.Sprintf("%s/api/v2/tenants/%s/databases/%s", c.baseURL, c.tenant, c.database)
}

// ResetCollection deletes and recreates the collection for a fresh session
func (c *Client) ResetCollection() error {
	// Delete by collection name directly
	req, err := http.NewRequest("DELETE",
		fmt.Sprintf("%s/collections/%s", c.base(), c.collection),
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to build delete request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete collection: %w", err)
	}
	defer resp.Body.Close()

	b, _ := io.ReadAll(resp.Body)
	// 200 = deleted, 404 = didn't exist — both are fine
	if resp.StatusCode != 200 && resp.StatusCode != 404 {
		return fmt.Errorf("delete collection failed (status %d): %s", resp.StatusCode, string(b))
	}

	// Recreate fresh empty collection
	return c.EnsureCollection()
}

// EnsureCollection creates the collection if it doesn't exist
func (c *Client) EnsureCollection() error {
	body, _ := json.Marshal(map[string]interface{}{
		"name": c.collection,
		"configuration": map[string]interface{}{
			"hnsw": map[string]string{"space": "cosine"},
		},
	})

	resp, err := c.httpClient.Post(
		c.base()+"/collections",
		"application/json",
		bytes.NewBuffer(body),
	)
	if err != nil {
		return fmt.Errorf("failed to create collection: %w", err)
	}
	defer resp.Body.Close()

	// 200/201 = created, 409 = already exists — both fine
	if resp.StatusCode != 200 && resp.StatusCode != 201 && resp.StatusCode != 409 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(b))
	}

	return nil
}

// getCollectionID fetches the internal UUID for the collection
func (c *Client) getCollectionID() (string, error) {
	resp, err := c.httpClient.Get(c.base() + "/collections/" + c.collection)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	id, ok := result["id"].(string)
	if !ok {
		return "", fmt.Errorf("could not get collection ID from response: %v", result)
	}
	return id, nil
}

// AddDocuments stores chunks + embeddings in ChromaDB using upsert
func (c *Client) AddDocuments(ids []string, embeddings [][]float64, texts []string, metadatas []map[string]string) error {
	collID, err := c.getCollectionID()
	if err != nil {
		return err
	}

	body, _ := json.Marshal(map[string]interface{}{
		"ids":        ids,
		"embeddings": embeddings,
		"documents":  texts,
		"metadatas":  metadatas,
	})

	resp, err := c.httpClient.Post(
		fmt.Sprintf("%s/collections/%s/upsert", c.base(), collID),
		"application/json",
		bytes.NewBuffer(body),
	)
	if err != nil {
		return fmt.Errorf("failed to add documents: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upsert failed (status %d): %s", resp.StatusCode, string(b))
	}

	return nil
}

// QueryResult holds a retrieved chunk
type QueryResult struct {
	Text    string
	DocName string
	Score   float64
}

// Query finds the most relevant chunks for a given embedding
func (c *Client) Query(queryEmbedding []float64, nResults int) ([]QueryResult, error) {
	collID, err := c.getCollectionID()
	if err != nil {
		return nil, err
	}

	body, _ := json.Marshal(map[string]interface{}{
		"query_embeddings": [][]float64{queryEmbedding},
		"n_results":        nResults,
		"include":          []string{"documents", "metadatas", "distances"},
	})

	resp, err := c.httpClient.Post(
		fmt.Sprintf("%s/collections/%s/query", c.base(), collID),
		"application/json",
		bytes.NewBuffer(body),
	)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Documents [][]string            `json:"documents"`
		Metadatas [][]map[string]string `json:"metadatas"`
		Distances [][]float64           `json:"distances"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var results []QueryResult
	if len(result.Documents) > 0 {
		for i, doc := range result.Documents[0] {
			docName := ""
			if len(result.Metadatas) > 0 && i < len(result.Metadatas[0]) {
				docName = result.Metadatas[0][i]["doc_name"]
			}
			score := 0.0
			if len(result.Distances) > 0 && i < len(result.Distances[0]) {
				score = result.Distances[0][i]
			}
			results = append(results, QueryResult{
				Text:    doc,
				DocName: docName,
				Score:   score,
			})
		}
	}

	return results, nil
}

// ListDocuments returns all unique document names stored
func (c *Client) ListDocuments() ([]string, error) {
	collID, err := c.getCollectionID()
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Post(
		fmt.Sprintf("%s/collections/%s/get", c.base(), collID),
		"application/json",
		bytes.NewBufferString(`{"include":["metadatas"]}`),
	)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Metadatas []map[string]string `json:"metadatas"`
	}
	json.NewDecoder(resp.Body).Decode(&result)

	seen := map[string]bool{}
	var docs []string
	for _, m := range result.Metadatas {
		if name, ok := m["doc_name"]; ok && !seen[name] {
			seen[name] = true
			docs = append(docs, name)
		}
	}

	return docs, nil
}
