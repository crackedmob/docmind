package rag

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// ExtractText detects file type and extracts text accordingly
func ExtractText(filePath string) (string, error) {
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".pdf":
		return extractPDFSmart(filePath)
	case ".txt":
		return extractTXT(filePath)
	case ".docx":
		return extractDOCX(filePath)
	case ".jpg", ".jpeg", ".png":
		fmt.Printf("🖼️  Running OCR on image...\n")
		return ExtractTextFromImage(filePath)
	default:
		return "", fmt.Errorf("unsupported file type: %s (supported: .pdf, .txt, .docx, .jpg, .png)", ext)
	}
}

// extractPDFSmart first tries normal text extraction.
// If no text is found or extraction fails, it falls back to OCR (scanned PDF)
func extractPDFSmart(pdfPath string) (string, error) {
	// Try normal text extraction first
	text, err := extractPDF(pdfPath)

	// Check if we got meaningful text (at least 50 characters)
	if err == nil && len(strings.TrimSpace(text)) > 500 {
		return text, nil
	}

	// Fallback to OCR — PDF is likely a scanned/photographed document
	fmt.Printf("⚠️  PDF appears to be image-based, switching to OCR...\n")
	ocrText, ocrErr := ExtractTextFromScannedPDF(pdfPath)
	if ocrErr != nil {
		return "", fmt.Errorf("both text extraction and OCR failed.\nText error: %v\nOCR error: %v", err, ocrErr)
	}
	return ocrText, nil
}

func extractPDF(pdfPath string) (string, error) {
	if _, err := os.Stat(pdfPath); err != nil {
		return "", fmt.Errorf("PDF file not found: %w", err)
	}
	tmpDir, err := os.MkdirTemp("", "docmind-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	conf := model.NewDefaultConfiguration()
	conf.ValidationMode = model.ValidationRelaxed

	err = api.ExtractContentFile(pdfPath, tmpDir, nil, conf)
	if err != nil {
		return "", fmt.Errorf("failed to extract PDF content: %w", err)
	}

	var builder strings.Builder
	files, err := filepath.Glob(filepath.Join(tmpDir, "*.txt"))
	if err != nil {
		return "", err
	}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		builder.Write(data)
		builder.WriteString("\n")
	}

	text := builder.String()
	if text == "" {
		return "", fmt.Errorf("no text could be extracted from PDF")
	}
	return text, nil
}

func extractTXT(filePath string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read text file: %w", err)
	}
	text := strings.TrimSpace(string(data))
	if text == "" {
		return "", fmt.Errorf("text file is empty")
	}
	return text, nil
}

type docxBody struct {
	Paragraphs []docxParagraph `xml:"body>p"`
}

type docxParagraph struct {
	Runs []docxRun `xml:"r"`
}

type docxRun struct {
	Text string `xml:"t"`
}

func extractDOCX(filePath string) (string, error) {
	r, err := zip.OpenReader(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open docx file: %w", err)
	}
	defer r.Close()

	for _, f := range r.File {
		if f.Name != "word/document.xml" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return "", fmt.Errorf("failed to open document.xml: %w", err)
		}
		defer rc.Close()

		data, err := io.ReadAll(rc)
		if err != nil {
			return "", fmt.Errorf("failed to read document.xml: %w", err)
		}

		var body docxBody
		if err := xml.Unmarshal(data, &body); err != nil {
			return stripXMLTags(string(data)), nil
		}

		var builder strings.Builder
		for _, para := range body.Paragraphs {
			for _, run := range para.Runs {
				builder.WriteString(run.Text)
			}
			builder.WriteString("\n")
		}

		text := strings.TrimSpace(builder.String())
		if text == "" {
			return "", fmt.Errorf("no text could be extracted from DOCX")
		}
		return text, nil
	}
	return "", fmt.Errorf("word/document.xml not found in docx file")
}

func stripXMLTags(input string) string {
	var builder strings.Builder
	inTag := false
	for _, r := range input {
		if r == '<' {
			inTag = true
		} else if r == '>' {
			inTag = false
			builder.WriteRune(' ')
		} else if !inTag {
			builder.WriteRune(r)
		}
	}
	lines := strings.Fields(builder.String())
	return strings.Join(lines, " ")
}

func ExtractTextFromPDF(pdfPath string) (string, error) {
	return extractPDF(pdfPath)
}
