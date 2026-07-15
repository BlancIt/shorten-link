package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"strings"

	_ "modernc.org/sqlite"
)

// -------------------------------------------- Storage --------------------------------------------

// Holds the original URL mappings in database (using SQLite).
type store struct {
	db *sql.DB
}

const createTableSQL = `
CREATE TABLE IF NOT EXISTS links (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	code       TEXT UNIQUE,
	original_url   TEXT NOT NULL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);`

// Initialize store
func newStore(path string) (*store, error) {
	db, err := sql.Open("sqlite", path)

	if err != nil {
		return nil, err
	}

	if _, err := db.Exec(createTableSQL); err != nil {
		return nil, err
	}

	return &store{db: db}, nil
}

// Stores a URL on database, get its code from the row id, and returns the code.
func (s *store) save(originalURL string) (string, error) {
	res, err := s.db.Exec(
		"INSERT INTO links (original_url) VALUES (?)",
		originalURL,
	)

	if err != nil {
		return "", err
	}

	id, err := res.LastInsertId()

	if err != nil {
		return "", err
	}

	code := toBase62(uint64(id))

	_, err = s.db.Exec(
		"UPDATE links SET code = ? WHERE id = ?",
		code, id,
	)

	if err != nil {
		return "", err
	}

	return code, nil
}

// Looks up a code. The bool is false if the code doesn't exist.
func (s *store) get(code string) (string, bool) {
	var originalURL string
	err := s.db.QueryRow(
		"SELECT original_url FROM links WHERE code = ?",
		code,
	).Scan(&originalURL)
	if err != nil {
		return "", false
	}
	return originalURL, true
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

// -------------------------------------------- Validation --------------------------------------------

// Reports whether a string is a valid URL (Must be http/https and have a host).
func isValidURL(s string) bool {
	u, err := url.ParseRequestURI(s)
	if err != nil {
		return false
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	if u.Host == "" {
		return false
	}

	return true
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

	req.URL = strings.TrimSpace(req.URL)

	if req.URL == "" {
		http.Error(w, "missing 'url' field", http.StatusBadRequest)
		return
	}

	if !isValidURL(req.URL) {
		http.Error(w, "invalid url: must be a valid http or https URL", http.StatusBadRequest)
		return
	}

	code, err := a.store.save(req.URL)

	if err != nil {
		http.Error(w, "could not save URL", http.StatusInternalServerError)
		return
	}

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
	originalURL, ok := a.store.get(code)
	if !ok {
		http.NotFound(w, r)
		return
	}
	http.Redirect(w, r, originalURL, http.StatusFound)
}

// -------------------------------------------- Main Program --------------------------------------------

// Uses mutex to ensure only one goroutine accesses the map at a time.
func main() {
	st, err := newStore("shorten.db")
	if err != nil {
		log.Fatal(err)
	}

	a := &app{store: st}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /shorten", a.shortenHandler)
	mux.HandleFunc("GET /{code}", a.redirectHandler)

	addr := ":8080"
	log.Printf("server listening on http://localhost%s", addr)

	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}
