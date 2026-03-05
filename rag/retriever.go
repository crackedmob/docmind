package rag

import (
	"fmt"
	"strings"

	"docmind/chroma"
)

// RetrievedContext holds the assembled context ready to send to Claude
type RetrievedContext struct {
	Text    string   // Full combined context string
	Sources []string // Unique document names used
}

// Retrieve takes a user question, embeds it, queries ChromaDB,
// and returns the most relevant chunks assembled as context
func Retrieve(question string, topK int) (*RetrievedContext, error) {
	if strings.TrimSpace(question) == "" {
		return nil, fmt.Errorf("question cannot be empty")
	}

	if topK <= 0 {
		topK = 5 // sensible default
	}

	// Step 1: Embed the question
	questionEmbedding, err := GetEmbedding(question)
	if err != nil {
		return nil, fmt.Errorf("failed to embed question: %w", err)
	}

	// Step 2: Query ChromaDB for nearest chunks
	client := chroma.NewClient("docmind")
	results, err := client.Query(questionEmbedding, topK)
	if err != nil {
		return nil, fmt.Errorf("vector search failed: %w", err)
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("no relevant chunks found — make sure you have uploaded documents")
	}

	// Step 3: Assemble context string + collect unique sources
	var contextBuilder strings.Builder
	seen := map[string]bool{}
	var sources []string

	for i, result := range results {
		contextBuilder.WriteString(fmt.Sprintf(
			"[Chunk %d | Source: %s]\n%s\n\n",
			i+1, result.DocName, result.Text,
		))

		if !seen[result.DocName] {
			seen[result.DocName] = true
			sources = append(sources, result.DocName)
		}
	}

	return &RetrievedContext{
		Text:    contextBuilder.String(),
		Sources: sources,
	}, nil
}

// RetrieveForDoc retrieves chunks filtered to a specific document name
// Useful when the user wants to query only one specific uploaded document
func RetrieveForDoc(question, docName string, topK int) (*RetrievedContext, error) {
	// Get broader results then filter — ChromaDB free tier doesn't support metadata filters easily
	ctx, err := Retrieve(question, topK*3)
	if err != nil {
		return nil, err
	}

	// Filter lines belonging to this doc
	var filtered strings.Builder
	var sources []string
	lines := strings.Split(ctx.Text, "\n\n")

	for _, block := range lines {
		if strings.Contains(block, "Source: "+docName) {
			filtered.WriteString(block)
			filtered.WriteString("\n\n")
		}
	}

	if filtered.Len() == 0 {
		return nil, fmt.Errorf("no chunks found for document '%s'", docName)
	}

	sources = append(sources, docName)

	return &RetrievedContext{
		Text:    filtered.String(),
		Sources: sources,
	}, nil
}
