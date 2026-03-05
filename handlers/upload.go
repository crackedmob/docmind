package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"docmind/chroma"
	"docmind/rag"
)

var chromaClient = chroma.NewClient("docmind")

// Collection is reset at startup by main.go

var supportedExtensions = map[string]bool{
	".pdf":  true,
	".txt":  true,
	".docx": true,
}

// UploadHandler handles file uploads, processes and stores them
func UploadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.ParseMultipartForm(50 << 20)

	file, header, err := r.FormFile("pdf") // keeping field name "pdf" for frontend compatibility
	if err != nil {
		jsonError(w, "Failed to read uploaded file: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if !supportedExtensions[ext] {
		jsonError(w, "Unsupported file type. Please upload a .pdf, .txt or .docx file", http.StatusBadRequest)
		return
	}

	// Save to temp file with correct extension
	tmpFile, err := os.CreateTemp("", "docmind-*"+ext)
	if err != nil {
		jsonError(w, "Failed to save file", http.StatusInternalServerError)
		return
	}
	defer os.Remove(tmpFile.Name())

	buf := make([]byte, 32*1024)
	for {
		n, err := file.Read(buf)
		if n > 0 {
			tmpFile.Write(buf[:n])
		}
		if err != nil {
			break
		}
	}
	tmpFile.Close()

	docName := strings.TrimSuffix(filepath.Base(header.Filename), ext)

	// Step 1: Extract text
	fmt.Printf("📄 Extracting text from: %s (%s)\n", docName, ext)
	text, err := rag.ExtractText(tmpFile.Name())
	if err != nil {
		jsonError(w, "Failed to extract text: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Step 2: Chunk text
	fmt.Printf("✂️  Chunking text...\n")
	chunks := rag.ChunkText(text, docName)
	fmt.Printf("   Created %d chunks\n", len(chunks))

	// Step 3: Embed chunks
	fmt.Printf("🔢 Generating embeddings...\n")
	embeddings, err := rag.EmbedChunks(chunks)
	if err != nil {
		jsonError(w, "Failed to generate embeddings: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Step 4: Store in ChromaDB
	fmt.Printf("💾 Storing in ChromaDB...\n")
	ids := make([]string, len(chunks))
	texts := make([]string, len(chunks))
	metadatas := make([]map[string]string, len(chunks))

	for i, chunk := range chunks {
		ids[i] = fmt.Sprintf("%s-chunk-%d", docName, chunk.Index)
		texts[i] = chunk.Text
		metadatas[i] = map[string]string{
			"doc_name":    docName,
			"chunk_index": fmt.Sprintf("%d", chunk.Index),
			"file_type":   ext,
		}
	}

	if err := chromaClient.AddDocuments(ids, embeddings, texts, metadatas); err != nil {
		jsonError(w, "Failed to store document: "+err.Error(), http.StatusInternalServerError)
		return
	}

	fmt.Printf("✅ Document '%s' processed successfully!\n", docName)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"doc_name": docName,
		"chunks":   len(chunks),
		"message":  fmt.Sprintf("Successfully processed '%s' into %d chunks", docName, len(chunks)),
	})
}

// ListDocumentsHandler returns all uploaded document names
func ListDocumentsHandler(w http.ResponseWriter, r *http.Request) {
	docs, err := chromaClient.ListDocuments()
	if err != nil {
		jsonError(w, "Failed to list documents: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"documents": docs,
	})
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
