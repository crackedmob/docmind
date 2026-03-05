package rag

import (
	"strings"
	"unicode"
)

const (
	ChunkSize    = 500 // words per chunk
	ChunkOverlap = 50  // overlapping words between chunks
)

// Chunk represents a piece of text with metadata
type Chunk struct {
	Text    string
	Index   int
	DocName string
}

// ChunkText splits a large text into overlapping chunks
func ChunkText(text, docName string) []Chunk {
	// Normalize whitespace
	text = strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return ' '
		}
		return r
	}, text)

	words := strings.Fields(text)
	var chunks []Chunk
	chunkIndex := 0

	for i := 0; i < len(words); i += ChunkSize - ChunkOverlap {
		end := i + ChunkSize
		if end > len(words) {
			end = len(words)
		}

		chunkWords := words[i:end]
		chunkText := strings.Join(chunkWords, " ")

		chunks = append(chunks, Chunk{
			Text:    chunkText,
			Index:   chunkIndex,
			DocName: docName,
		})

		chunkIndex++

		// Stop if we've reached the end
		if end == len(words) {
			break
		}
	}

	return chunks
}
