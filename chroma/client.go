// this file is a translator between go backend and chromaDb.
// as chromaDB doesn't have Go lirary, this file manually speaks to it over HTTP
// like writing our own mini API client from scratch
package chroma

// imports these are
import (
	"bytes"         // used to convert data into a format that can be sent over HTTP
	"encoding/json" // converts Go structs/maps into JSON(and back) since chromaDB communicates in JSON
	"fmt"           //string formatting, used for building URLs and error messages
	"io"            // reads raw HTTP response bodies
	"net/http"      // Go's built-in HTTP client, used to make GET/POST/DELETE requests to chromaDB
)

// these values tell the client where chromaDB lives and which workspace to use
const (
	chromaURL      = "http://localhost:8000" // chromaDB runs locally inside docker on port 8000
	chromaTenant   = "default_tenant"
	chromaDatabase = "default_database"
)// the other two: chromaDB v2 organizes data in hierarchy
// tenant->database->collection. 
// tenant is a company, database is a project, collection is a table
// to make it more easy and understandable :
// gDrive(whole system) -> my account is tenant -> the project folder is database -> the docmind is collection
// we use it as default because chromaDb was designed to support multiple users
// and multiple projects on the same server. We use default because DocMind is a single user, single project app.
// and we dont have any need for multiple tenants or databases
// so we just use the ones chromaDB creates automatically - default_tenant and default_database-
// and go staraight to our collection docmind

// Client wraps ChromaDB HTTP API (v2)
type Client struct { // struct in Go is like a class - it groups related data together
	// baseURL, tenant, database, collection : these are the addressing information
	// for where to store and find data in chromaDB
	baseURL    string 
	tenant     string
	database   string
	collection string
	httpClient *http.Client // it's the actual HTTP connection object that send requests
	// having it here means all functions share one connection instead of creating a new one every time
}
// this is a constructor - the function we call to create a new client.
// the * means it return a pointer(memory address) to the client rather than a copy.
// it's important because we want all parts of the app sharing the same client instance, not different copies.
// NewClient creates a new ChromaDB v2 client
func NewClient(collection string) *Client {
	return &Client{
		baseURL:    chromaURL,
		tenant:     chromaTenant,
		database:   chromaDatabase,
		collection: collection,
		httpClient: &http.Client{},
	} // when called as chroma.NewClient("docmind"), it creates a client pointing
	  // at the docming collection inside the chromaDB
}
// base() - the URL builder:
// it is a helper function that builds the base URL for every API call.
// instead of repeating the long URL, everywhere, every function calls c.base()
// and appends whatever endpoint it needs
// base path for all v2 API calls
func (c *Client) base() string {
	return fmt.Sprintf("%s/api/v2/tenants/%s/databases/%s", c.baseURL, c.tenant, c.database) // this is specific to chromaDB v2's API format - the tenant and database must be in every URL
}
// in DocMind we have a session thing 
// this function is called everytime DocMind starts up - it gives the fresh session behaviour
// it builds a DELETE HTTP request targetting the collection URL
// ResetCollection deletes and recreates the collection for a fresh session
func (c *Client) ResetCollection() error {
	// Delete by collection name directly
	// http.NewRequest returns two things simulatneously:
	// req - the actual DELETE request object if it was built successfully
	// err - an error object if something went wrong, or nil if everything is fine
	req, err := http.NewRequest("DELETE",
		fmt.Sprintf("%s/collections/%s", c.base(), c.collection),
		nil,
	) // the nil at the end means no request body (DELETE requests don't need one)
	// nil means "nothing"/"no error"
	if err != nil { // checks if an error actually happened 
		return fmt.Errorf("failed to build delete request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete collection: %w", err)
	}
	defer resp.Body.Close() // defer = run this when the function exits no matter what
	//it is very Go-specific. HTTP responses must be manually closed or we get memory leaks

	b, _ := io.ReadAll(resp.Body)
	// 200 = deleted, 404 = didn't exist — both are fine since on first startup there is nothing to delete
	if resp.StatusCode != 200 && resp.StatusCode != 404 {
		return fmt.Errorf("delete collection failed (status %d): %s", resp.StatusCode, string(b))
	}
	// after deleting, it immediately calls EnsureCollection() to create a fresh empty one
	// Recreate fresh empty collection
	return c.EnsureCollection()
}

// EnsureCollection creates the collection if it doesn't exist
func (c *Client) EnsureCollection() error {
	body, _ := json.Marshal(map[string]interface{}{ // collection config as JSON byte slice
		"name": c.collection,
		"configuration": map[string]interface{}{
			"hnsw": map[string]string{"space": "cosine"},
		}, // HNSW is hierarchical navigable small world - it is the algorithm 
		   // chromaDB uses to find similar vectors quickly.
		   // cosine similarity is the mathematical method used to measure how similar
		   // two vectors are. It measures the angle between them - 0 degree angle between them means identical meaning,
		   // 90 degree angle between them means completely unrelated.
		   // standard for text embeddings
	})
	// this part actually sends the request to chromaDB to create a new collection
	// httpClient.Post expects an io.Reader type, not raw bytes
	resp, err := c.httpClient.Post( // sends an HTTP POST request
		c.base()+"/collections", // where to send it , builds the full URL by combining the base URL with collection, this is chromaDB's endpoint for creating a new collection
		"application/json", // what format the data is in, this is a content-type header, it tells the chromaDB about the type of content it has. Without this chromaDB would not know how to read the request body and would rehect it
		bytes.NewBuffer(body), // the actual data being sent, it wraps the bytes into a readable stream
		// that the HTTP client can read from while sending
	)
	if err != nil {
		return fmt.Errorf("failed to create collection: %w", err)
	}
	// when chromaDB send back a response, Go opens a network stream to read it.
	// that stream stays open and holds memory until we explicitly close it
	// if we never close it, we get a memory leak
	defer resp.Body.Close()

	// 200/201 = created, 409 = already exists — both fine
	if resp.StatusCode != 200 && resp.StatusCode != 201 && resp.StatusCode != 409 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(b))
	}

	return nil
}

// getCollectionID fetches the internal UUID that chromaDB assigns to a collection
// chromaDB v2 requires this UUID for all write operations(query, delete)
// we cannot use the human-readable collection name ("docmind") directly 
// for these - we must first look up its internal ID, which looks like : "a3f2c1d4-8b7e-4c2a-..."
func (c *Client) getCollectionID() (string, error) {
	// send a GET request to chromaDB asking for details about our collection by name
	// base() URL + collection, gets us a new URL for out collection "docmind"
	// chromaDB responds with a  JSON object containing the collection's metadata including its UUID
	resp, err := c.httpClient.Get(c.base() + "/collections/" + c.collection)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close() // always close the response body to prevent the memory leaks

	// declared the variable to hold the decoded JSON response
	// map[string]interface{} is Go's way of saying "a JSON object where keys are strings
	// and values can be anything - string, number, boolean, nested object, etc."
	// we use this instead of a specific struct because we only need one field (id)
	// and don't want to define a full struct just for that
	var result map[string]interface{}
	
	// json.NewDecoder(resp.Body) creates a JSON decoder that reads directly from
	// the HTTP response body stream - more efficient than reading all bytes first
	// .Decode(&result) parses the JSON and populates our result map
	// the & means we are passing a pointer so Decode can write into our variable
	// after it the result might look like:
	// {
	// 		"id": "a3f2c1d4-8b7e-9f1b-3e5d7c8a2b4e",
	//		"name": "docmind",
	//		"tenant": "default_tenant",
	//		"database": "default_database"
	// }
	json.NewDecoder(resp.Body).Decode(&result)

	// result["id"] fetches the value under the "id" key from the map.
	// however, since our map is map[string]interface{}, Go doesn't know the type 
	// of the value yet - it just knows it's "some interface"
	// .(string) is a TYPE ASSERATION - it says "I believe this value is a string,
	// please extract it as one."
	// this returns two values:
	//		id -> the string value if the assertion succeeded ("a3f2c1d4-...")
	//		ok -> true if it was indeed a string, false if it wasn't or the key didn't exist
	// it is safer than result["id"].(string) alone, which wwould PANIC (crash)
	// if the key is missing or the value isn't a string
	id, ok := result["id"].(string)

	// if ok is false, either:
	// 1. the "id" key doesn't exist in the response (collection wasn't found)
	// 2. the value exists but isn't a string (unexpected API response)
	// either way we can't proceed, so return an empty string and an error.
	if !ok {
		return "", fmt.Errorf("could not get collection ID from response: %v", result)
	}
	// everything went well - return the UUID string and nil (meaning no error).
	// nil is Go's way of saying "there is no error".
	// the called (AddDocuments, Query, etc.) will use this ID to build
	// endpoints like : /collections/a3f2c1d4-.../upsert
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
