package main

import (
	"fmt"
	"log"
	"net/http"

	"docmind/chroma"
	"docmind/handlers"

	"github.com/joho/godotenv"
)

func main() {
	// Load .env file
	if err := godotenv.Load(); err != nil {
		fmt.Println("⚠️  No .env file found, falling back to system environment variables")
	} else {
		fmt.Println("✅ Loaded environment variables from .env")
	}

	// Reset ChromaDB collection on every startup — fresh session
	fmt.Println("🔄 Starting fresh session, clearing previous documents...")
	client := chroma.NewClient("docmind")
	if err := client.ResetCollection(); err != nil {
		fmt.Println("⚠️  Could not reset ChromaDB:", err)
		fmt.Println("   Make sure ChromaDB is running: docker run -p 8000:8000 chromadb/chroma")
	} else {
		fmt.Println("✅ Session ready — all previous documents cleared")
	}

	// Serve static frontend
	fs := http.FileServer(http.Dir("./static"))
	http.Handle("/", fs)

	// API routes
	http.HandleFunc("/api/upload", handlers.UploadHandler)
	http.HandleFunc("/api/query", handlers.QueryHandler)
	http.HandleFunc("/api/summarize", handlers.SummarizeHandler)
	http.HandleFunc("/api/documents", handlers.ListDocumentsHandler)

	fmt.Println("🚀 DocMind running at http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
