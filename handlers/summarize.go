package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"docmind/llm"
	"docmind/rag"
)

type summarizeRequest struct {
	DocNames []string `json:"doc_names"` // 1 doc = summarize, 2 docs = compare
}

// SummarizeHandler summarizes a document or compares two documents
func SummarizeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req summarizeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if len(req.DocNames) == 0 || len(req.DocNames) > 2 {
		jsonError(w, "Provide 1 document to summarize or 2 to compare", http.StatusBadRequest)
		return
	}

	// Fetch chunks for each document by querying with a generic embedding
	docContents := make(map[string]string)

	for _, docName := range req.DocNames {
		// Use a broad query to get representative chunks
		embedding, err := rag.GetEmbedding("main topics key findings summary overview")
		if err != nil {
			jsonError(w, "Embedding failed: "+err.Error(), http.StatusInternalServerError)
			return
		}

		results, err := chromaClient.Query(embedding, 10)
		if err != nil {
			jsonError(w, "Retrieval failed: "+err.Error(), http.StatusInternalServerError)
			return
		}

		// Filter results to only this document
		var docChunks []string
		for _, res := range results {
			if res.DocName == docName {
				docChunks = append(docChunks, res.Text)
			}
		}

		if len(docChunks) == 0 {
			jsonError(w, fmt.Sprintf("Document '%s' not found", docName), http.StatusNotFound)
			return
		}

		docContents[docName] = strings.Join(docChunks, "\n\n")
	}

	var systemPrompt, userPrompt, resultType string

	if len(req.DocNames) == 1 {
		// Summarize single document
		resultType = "summary"
		systemPrompt = `You are DocMind, an expert document summarizer.
Create a clear, structured summary with:
- Main topic/purpose
- Key findings or arguments
- Important details
- Conclusions`

		userPrompt = fmt.Sprintf(`Please summarize this document titled "%s":

%s`, req.DocNames[0], docContents[req.DocNames[0]])

	} else {
		// Compare two documents
		resultType = "comparison"
		systemPrompt = `You are DocMind, an expert at comparing academic and research documents.
Compare the two documents covering:
- Main topics and goals of each
- Key similarities
- Key differences  
- Which is more comprehensive on specific aspects`

		userPrompt = fmt.Sprintf(`Compare these two documents:

=== Document 1: "%s" ===
%s

=== Document 2: "%s" ===
%s

Provide a detailed comparison.`,
			req.DocNames[0], docContents[req.DocNames[0]],
			req.DocNames[1], docContents[req.DocNames[1]])
	}

	fmt.Printf("📝 %s for: %v\n", resultType, req.DocNames)

	result, err := llm.Ask(systemPrompt, userPrompt)
	if err != nil {
		jsonError(w, "Claude API failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"type":      resultType,
		"doc_names": req.DocNames,
		"result":    result,
	})
}
