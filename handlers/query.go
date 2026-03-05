package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"docmind/llm"
	"docmind/rag"
)

type chatMessage struct {
	Role    string `json:"role"` // "user" or "assistant"
	Content string `json:"content"`
}

type queryRequest struct {
	Question string        `json:"question"`
	DocName  string        `json:"doc_name"` // optional: filter by doc
	History  []chatMessage `json:"history"`  // previous messages
}

// QueryHandler handles user questions via RAG pipeline with chat history
func QueryHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req queryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(req.Question) == "" {
		jsonError(w, "Question cannot be empty", http.StatusBadRequest)
		return
	}

	fmt.Printf("❓ Query: %s\n", req.Question)

	// Step 1 & 2: Embed question + retrieve relevant chunks
	var ctx *rag.RetrievedContext
	var err error

	if req.DocName != "" {
		ctx, err = rag.RetrieveForDoc(req.Question, req.DocName, 3)
	} else {
		ctx, err = rag.Retrieve(req.Question, 3)
	}

	if err != nil {
		jsonError(w, "Failed to retrieve context: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Step 3: Build system prompt with document context
	systemPrompt := fmt.Sprintf(`You are DocMind, an intelligent document assistant.
You answer questions based ONLY on the provided document context below.
If the answer is not found in the context, say so clearly.
Always cite which document/source your answer comes from.
You also remember the conversation history and can answer follow-up questions.

Document context:
%s`, ctx.Text)

	// Step 4: Build history for Groq (convert our format to llm format)
	var history []llm.Message
	for _, msg := range req.History {
		history = append(history, llm.Message{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}

	fmt.Printf("🤖 Asking Groq (history: %d messages)...\n", len(history))
	answer, err := llm.AskWithHistory(systemPrompt, req.Question, history)
	if err != nil {
		jsonError(w, "Failed to get answer: "+err.Error(), http.StatusInternalServerError)
		return
	}

	_ = strings.TrimSpace

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"answer":  answer,
		"sources": ctx.Sources,
	})
}
