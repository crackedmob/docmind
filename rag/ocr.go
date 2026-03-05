package rag

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// getGhostscriptPath returns the correct ghostscript executable path
func getGhostscriptPath() string {
	if envPath := os.Getenv("GS_PATH"); envPath != "" {
		return envPath
	}
	if runtime.GOOS == "windows" {
		candidates := []string{
			`C:\Program Files\gs\gs10.06.0\bin\bin\gswin64c.exe`,
			`C:\Program Files\gs\gs10.05.0\bin\bin\gswin64c.exe`,
			`C:\Program Files\gs\gs10.04.0\bin\bin\gswin64c.exe`,
		}
		for _, path := range candidates {
			if _, err := os.Stat(path); err == nil {
				return path
			}
		}
		return "gswin64c" // fallback to PATH
	}
	return "gs"
}

// getTesseractPath returns the correct tesseract executable path
func getTesseractPath() string {
	if envPath := os.Getenv("TESSERACT_PATH"); envPath != "" {
		return envPath
	}
	if runtime.GOOS == "windows" {
		candidates := []string{
			`C:\Program Files\Tesseract-OCR\tesseract.exe`,
			`C:\Program Files (x86)\Tesseract-OCR\tesseract.exe`,
		}
		for _, path := range candidates {
			if _, err := os.Stat(path); err == nil {
				return path
			}
		}
		return "tesseract"
	}
	return "tesseract"
}

// ExtractTextFromImage runs Tesseract OCR on an image file
func ExtractTextFromImage(imagePath string) (string, error) {
	tmpDir, err := os.MkdirTemp("", "docmind-tess-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	outBase := filepath.Join(tmpDir, "result")
	tessPath := getTesseractPath()
	fmt.Printf("   Using Tesseract: %s\n", tessPath)

	cmd := exec.Command(tessPath, imagePath, outBase, "-l", "eng")
	out, err := cmd.CombinedOutput()
	fmt.Printf("   Tesseract output: %s\n", string(out))
	if err != nil {
		return "", fmt.Errorf("tesseract failed: %w\nOutput: %s", err, string(out))
	}

	resultFile := outBase + ".txt"
	fmt.Printf("   Reading result from: %s\n", resultFile)

	data, err := os.ReadFile(resultFile)
	if err != nil {
		return "", fmt.Errorf("failed to read OCR output file: %w", err)
	}

	text := strings.TrimSpace(string(data))
	fmt.Printf("   Extracted %d characters\n", len(text))

	if text == "" {
		return "", fmt.Errorf("no text found in image")
	}

	return text, nil
}

// ExtractTextFromScannedPDF converts PDF pages to images then runs OCR
func ExtractTextFromScannedPDF(pdfPath string) (string, error) {
	tmpDir, err := os.MkdirTemp("", "docmind-gs-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	gsPath := getGhostscriptPath()
	outputPattern := filepath.Join(tmpDir, "page-%03d.png")

	fmt.Printf("   Using Ghostscript: %s\n", gsPath)
	fmt.Printf("   Output pattern: %s\n", outputPattern)

	gsCmd := exec.Command(gsPath,
		"-dBATCH",
		"-dNOPAUSE",
		"-dQUIET",
		"-sDEVICE=png16m",
		"-r300",
		"-sOutputFile="+outputPattern,
		pdfPath,
	)

	out, err := gsCmd.CombinedOutput()
	fmt.Printf("   Ghostscript output: %s\n", string(out))
	if err != nil {
		return "", fmt.Errorf("ghostscript failed: %w\nOutput: %s", err, string(out))
	}

	pages, err := filepath.Glob(filepath.Join(tmpDir, "page-*.png"))
	if err != nil {
		return "", fmt.Errorf("glob failed: %w", err)
	}

	fmt.Printf("   Found %d pages\n", len(pages))
	if len(pages) == 0 {
		// List what IS in tmpDir for debugging
		entries, _ := os.ReadDir(tmpDir)
		fmt.Printf("   tmpDir contents: ")
		for _, e := range entries {
			fmt.Printf("%s ", e.Name())
		}
		fmt.Println()
		return "", fmt.Errorf("no pages generated from PDF")
	}

	sort.Strings(pages)

	var builder strings.Builder
	for i, page := range pages {
		fmt.Printf("   OCR page %d/%d: %s\n", i+1, len(pages), page)
		text, err := ExtractTextFromImage(page)
		if err != nil {
			fmt.Printf("   ⚠️  Page %d failed: %v\n", i+1, err)
			continue
		}
		if text != "" {
			builder.WriteString(text)
			builder.WriteString("\n\n")
		}
	}

	result := strings.TrimSpace(builder.String())
	if result == "" {
		return "", fmt.Errorf("no text could be extracted from scanned PDF")
	}

	return result, nil
}
