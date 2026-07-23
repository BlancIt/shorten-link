package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// -------------------------------------------- Storage --------------------------------------------

// Holds the original URL mappings in database (using SQLite).
type store struct {
	db *sql.DB
}

// Holds the info returned by GET /stats/{code}.
type stats struct {
	Code        string     `json:"code"`
	OriginalURL string     `json:"original_url"`
	Clicks      int        `json:"clicks"`
	CreatedAt   time.Time  `json:"created_at"`
	ExpiresAt   *time.Time `json:"expires_at"` // null if never expires
}

// Query for initializing links table
const createTableSQL = `
CREATE TABLE IF NOT EXISTS links (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	code       TEXT UNIQUE,
	original_url   TEXT NOT NULL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	expires_at DATETIME,
	clicks     INTEGER NOT NULL DEFAULT 0
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
func (s *store) save(originalURL string, expiresAt *time.Time) (string, error) {
	res, err := s.db.Exec(
		"INSERT INTO links (original_url, expires_at) VALUES (?, ?)",
		originalURL, expiresAt,
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

// Stores a URL on database with specific/custom code.
func (s *store) saveWithCode(code, originalURL string, expiresAt *time.Time) error {
	_, err := s.db.Exec(
		"INSERT INTO links (code, original_url, expires_at) VALUES (?, ?, ?)",
		code, originalURL, expiresAt,
	)

	return err
}

// Looks up a code. The bool is false if the code doesn't exist.
func (s *store) get(code string) (string, bool) {
	var originalURL string
	var expiresAt *time.Time

	err := s.db.QueryRow(
		"SELECT original_url, expires_at FROM links WHERE code = ?",
		code,
	).Scan(&originalURL, &expiresAt)

	if err != nil {
		return "", false // not found
	}

	if expiresAt != nil && time.Now().After(*expiresAt) {
		return "", false
	}
	
	return originalURL, true
}

// Increases the click count for a code.
func (s *store) incrementClicks(code string) {
	_, _ = s.db.Exec("UPDATE links SET clicks = clicks + 1 WHERE code = ?", code)
}

// Returns stats for a code; false if it doesn't exist.
func (s *store) getStats(code string) (stats, bool) {
	var st stats
	err := s.db.QueryRow(
		"SELECT code, original_url, clicks, created_at, expires_at FROM links WHERE code = ?",
		code,
	).Scan(&st.Code, &st.OriginalURL, &st.Clicks, &st.CreatedAt, &st.ExpiresAt)

	if err != nil {
		return stats{}, false
	}

	return st, true
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

// Checks whether a string is a valid URL (Must be http/https and have a host).
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

// Checks whether an alias is safe to use as a short code.
func isValidAlias(s string) bool {
	if len(s) < 5 || len(s) > 20 {
		return false
	}

	for _, r := range s {
		isLetter := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
		isDigit := r >= '0' && r <= '9'
		if !isLetter && !isDigit && r != '-' && r != '_' {
			return false
		}
	}
	
	return true
}

// -------------------------------------------- HTTP handlers --------------------------------------------

// JSON request from POST /shorten.
type shortenRequest struct {
	URL       string `json:"url"`
	Alias     string `json:"alias"`
	ExpiresIn int    `json:"expires_in"` // optional
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
	var expiresAt *time.Time
	var code string

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

	if req.ExpiresIn > 0 {
		t := time.Now().Add(time.Duration(req.ExpiresIn) * time.Second)
		expiresAt = &t
	}

	if req.Alias != "" {
		if !isValidAlias(req.Alias) {
			http.Error(w, "invalid alias: 5-20 chars, letters/digits/-/_ only", http.StatusBadRequest)
			return
		}

		if err := a.store.saveWithCode(req.Alias, req.URL, expiresAt); err != nil {
			http.Error(w, "alias already taken", http.StatusConflict) // 409
			return
		}
		code = req.Alias
	} else {
		var err error
		code, err = a.store.save(req.URL, expiresAt)

		if err != nil {
			http.Error(w, "could not save URL", http.StatusInternalServerError)
			return
		}
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
	code := r.PathValue("code")
	originalURL, ok := a.store.get(code)

	if !ok {
		http.NotFound(w, r)
		return
	}

	a.store.incrementClicks(code)  
	http.Redirect(w, r, originalURL, http.StatusFound)
}

// Handles GET /stats/{code}.
func (a *app) statsHandler(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	st, ok := a.store.getStats(code)

	if !ok {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(st)
}

// -------------------------------------------- Main Program --------------------------------------------

func main() {
	st, err := newStore("shorten.db")
	if err != nil {
		log.Fatal(err)
	}

	a := &app{store: st}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /shorten", a.shortenHandler)
	mux.HandleFunc("GET /{code}", a.redirectHandler)
	mux.HandleFunc("GET /stats/{code}", a.statsHandler)

	addr := ":8080"
	log.Printf("server listening on http://localhost%s", addr)

	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}
