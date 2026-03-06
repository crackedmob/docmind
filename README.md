# DocMind

A RAG (Retrieval-Augmented Generation) system built in Go that lets you upload documents and have an intelligent conversation with them. Answers are grounded in your actual documents — not AI guesswork.

Built as a final year project by Shipra Sharma.

---

## What is RAG?

RAG stands for **Retrieval-Augmented Generation**. Instead of asking an AI to answer from memory, RAG:

1. Breaks your documents into chunks
2. Converts them into vectors (embeddings)
3. Stores them in a vector database
4. When you ask a question, finds the most relevant chunks
5. Sends those chunks + your question to an LLM
6. The LLM answers using only your document content

This means answers are accurate, sourced, and specific to your files.

---

## Features

- **Multi-format support** — PDF, DOCX, TXT, JPG, PNG
- **OCR** — Scanned PDFs and photographed documents via Tesseract + Ghostscript
- **Chat with follow-up questions** — Full conversation history sent to the LLM
- **Summarize & Compare** — Summarize one document or compare two side by side
- **Session-based** — Every restart wipes the slate clean, no leftover documents
- **Theme switcher** — 5 pastel colour themes (Sand, Rose, Sky, Lavender, Mint)
- **Typewriter UI** — Built with Courier Prime + Special Elite fonts

---

## Tech Stack

| Layer | Technology |
|---|---|
| Backend | Go 1.21+ |
| Vector Database | ChromaDB (Docker) |
| Embeddings | Google Gemini API (`gemini-embedding-001`) |
| LLM | Groq API (`llama-3.3-70b-versatile`) |
| OCR | Tesseract v5 + Ghostscript |
| Frontend | Vanilla HTML / CSS / JS |

---

## Project Structure

```
docmind/
│
├── .env                  ← API keys (never commit this)
├── .gitignore
├── go.mod
├── main.go               ← Entry point, starts server, resets session
│
├── handlers/
│   ├── upload.go         ← File upload → parse → chunk → embed → store
│   ├── query.go          ← Question → retrieve → LLM → answer
│   └── summarize.go      ← Summarize or compare documents
│
├── rag/
│   ├── parser.go         ← Detects file type, routes to correct extractor
│   ├── ocr.go            ← OCR via Tesseract CLI + Ghostscript
│   ├── chunker.go        ← Splits text into overlapping chunks
│   ├── embedder.go       ← Converts chunks to vectors via Gemini
│   └── retriever.go      ← Queries ChromaDB, assembles context
│
├── chroma/
│   └── client.go         ← ChromaDB HTTP client (v2 API)
│
├── llm/
│   └── claude.go         ← Groq API client with chat history support
│
└── static/
    └── index.html        ← Full frontend (landing page + app)
```

---

## Prerequisites

Before running DocMind, install the following:

### 1. Go
Download from https://golang.org/dl/ (v1.21 or higher)

### 2. Docker
Download from https://www.docker.com/products/docker-desktop

### 3. Tesseract OCR (for scanned documents)
- Windows: https://github.com/UB-Mannheim/tesseract/wiki
- Add `C:\Program Files\Tesseract-OCR` to system PATH

### 4. Ghostscript (for scanned PDFs)
- Windows: https://www.ghostscript.com/releases/gsdnld.html
- Add the `bin\bin` folder to system PATH (e.g. `C:\Program Files\gs\gs10.06.0\bin\bin`)

---

## Setup

### 1. Clone the repository

```bash
git clone https://github.com/crackedmob/docmind.git
cd docmind
```

### 2. Get free API keys

**Groq** (LLM — completely free):
- Go to https://console.groq.com
- Sign up → API Keys → Create key

**Google Gemini** (Embeddings — free tier):
- Go to https://aistudio.google.com
- Sign in with Google → Get API key

### 3. Create your `.env` file

Create a file called `.env` in the root of the project:

```env
GROQ_API_KEY=your_groq_api_key_here
GEMINI_API_KEY=your_gemini_api_key_here

# Optional — only needed if Ghostscript isn't in PATH
GS_PATH=C:\Program Files\gs\gs10.06.0\bin\bin\gswin64c.exe

# Optional — only needed if Tesseract isn't in PATH
TESSERACT_PATH=C:\Program Files\Tesseract-OCR\tesseract.exe
```

### 4. Start ChromaDB

```bash
docker run -d -p 8000:8000 chromadb/chroma
```

### 5. Install Go dependencies

```bash
go mod tidy
```

### 6. Run DocMind

```bash
go run main.go
```

Open your browser at **http://localhost:8080**

---

## Usage

### Uploading Documents
1. Click **Upload** tab
2. Drag and drop or click to browse
3. Supported: PDF, DOCX, TXT, JPG, PNG (up to 50MB)
4. Wait for processing — you'll see chunk count when done

### Chatting with Documents
1. Click **Chat** tab
2. Type your question and press Enter
3. Ask follow-up questions — DocMind remembers the conversation
4. Source documents are shown as pills under each answer

### Summarizing & Comparing
1. Click **Summarize** tab
2. Select one document → click **Summarize**
3. Select two documents → click **Compare Both**

---

## API Endpoints

| Method | Endpoint | Description |
|---|---|---|
| POST | `/api/upload` | Upload a file (multipart/form-data, field: `pdf`) |
| POST | `/api/query` | Ask a question `{ question, history[] }` |
| POST | `/api/summarize` | Summarize/compare `{ doc_names: [] }` |
| GET | `/api/documents` | List uploaded documents in current session |

---

## Rate Limits

DocMind uses Groq's free tier which has a limit of **12,000 tokens per minute**. If you hit this limit you'll see a rate limit error — simply wait 15 seconds and try again.

To reduce token usage, DocMind retrieves 3 chunks per query by default. For broad questions use the Summarize tab instead of the Chat tab.

---

## How the RAG Pipeline Works

```
Upload
  ↓
Extract text (PDF / DOCX / TXT / OCR)
  ↓
Split into 500-word chunks with 50-word overlap
  ↓
Convert each chunk to a 768-dimension vector via Gemini
  ↓
Store vectors + text + metadata in ChromaDB

Query
  ↓
Convert question to vector via Gemini
  ↓
Find top 3 most similar vectors in ChromaDB (cosine similarity)
  ↓
Assemble retrieved chunks as context
  ↓
Send context + conversation history + question to Groq LLaMA
  ↓
Return answer + source document names
```

---

## Troubleshooting

**`refused to connect` in browser**
- Make sure Docker is running (ChromaDB must be active)
- Try `http://127.0.0.1:8080` instead of `localhost`

**`Groq API error: Rate limit reached`**
- Wait 15 seconds and try again
- Free tier limit is 12,000 tokens/minute

**`no text could be extracted from scanned PDF`**
- Verify Tesseract: `tesseract --version`
- Verify Ghostscript: `gswin64c --version` (Windows)
- Add `TESSERACT_PATH` and `GS_PATH` to your `.env` file

**Old documents showing after restart**
- This shouldn't happen — DocMind wipes ChromaDB on every startup
- If it does, run `docker restart <container_id>` to fully reset ChromaDB

---

## License

MIT License — free to use, modify and distribute.

---

*DocMind — built with Go, ChromaDB, Groq and Gemini.*