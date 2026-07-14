package main

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
)

// -------------------------------------------- Storage --------------------------------------------

// Holds the original URL mappings in memory.
type store struct {
	mu      sync.Mutex
	urls    map[string]string // code -> long URL
	counter uint64            // incrementing ID, encoded to base62 for the code
}

// Initialize store
func newStore() *store {
	return &store{urls: make(map[string]string)}
}

// Stores a URL under a freshly generated code and returns it. Uses mutex to ensure only one goroutine accesses the map at a time.
func (s *store) save(longURL string) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.counter++
	code := toBase62(s.counter)
	s.urls[code] = longURL
	return code
}

// Looks up a code. The bool is false if the code doesn't exist. Also uses mutex.
func (s *store) get(code string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	longURL, ok := s.urls[code]
	return longURL, ok
}

// -------------------------------------------- Base62 encoding --------------------------------------------

const base62Alphabet = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

// Converts counter to short string to make it URL-friendly
func toBase62(n uint64) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{base62Alphabet[n%62]}, b...)
		n /= 62
	}
	return string(b)
}

// -------------------------------------------- HTTP handlers --------------------------------------------

// JSON request from POST /shorten.
type shortenRequest struct {
	URL string `json:"url"`
}

// JSON response for the shortened url link.
type shortenResponse struct {
	Code     string `json:"code"`
	ShortURL string `json:"short_url"`
}

// Bundles the dependencies for the handlers (just the store for now).
type app struct {
	store *store
}

// Handles POST /shorten.
func (a *app) shortenHandler(w http.ResponseWriter, r *http.Request) {
	var req shortenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if req.URL == "" {
		http.Error(w, "missing 'url' field", http.StatusBadRequest)
		return
	}

	code := a.store.save(req.URL)

	resp := shortenResponse{
		Code:     code,
		ShortURL: "http://localhost:8080/" + code,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// Handles GET /{code} and redirects to the original URL.
func (a *app) redirectHandler(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code") // pulls "{code}" out of the route pattern
	longURL, ok := a.store.get(code)
	if !ok {
		http.NotFound(w, r)
		return
	}
	http.Redirect(w, r, longURL, http.StatusFound)
}

// -------------------------------------------- Main Program -------------------------------------------- 

func main() {
	a := &app{store: newStore()}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /shorten", a.shortenHandler)
	mux.HandleFunc("GET /{code}", a.redirectHandler)

	addr := ":8080"
	log.Printf("server listening on http://localhost%s", addr)

	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}
