package store

import (
	"database/sql"
	"time"

	"shorten-link/internal/base62"

	_ "modernc.org/sqlite"
)


// Holds the original URL mappings in database (using SQLite).
type Store struct {
	db *sql.DB
}

// Holds the info returned by GET /stats/{code}.
type Stats struct {
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
func New(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)

	if err != nil {
		return nil, err
	}

	if _, err := db.Exec(createTableSQL); err != nil {
		return nil, err
	}

	return &Store{db: db}, nil
}

// Stores a URL on database, get its code from the row id, and returns the code.
func (s *Store) Save(originalURL string, expiresAt *time.Time) (string, error) {
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

	code := base62.Encode(uint64(id))

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
func (s *Store) SaveWithCode(code, originalURL string, expiresAt *time.Time) error {
	_, err := s.db.Exec(
		"INSERT INTO links (code, original_url, expires_at) VALUES (?, ?, ?)",
		code, originalURL, expiresAt,
	)

	return err
}

// Looks up a code. The bool is false if the code doesn't exist.
func (s *Store) Get(code string) (string, bool) {
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
func (s *Store) IncrementClicks(code string) {
	_, _ = s.db.Exec("UPDATE links SET clicks = clicks + 1 WHERE code = ?", code)
}

// Returns stats for a code; false if it doesn't exist.
func (s *Store) GetStats(code string) (Stats, bool) {
	var st Stats
	err := s.db.QueryRow(
		"SELECT code, original_url, clicks, created_at, expires_at FROM links WHERE code = ?",
		code,
	).Scan(&st.Code, &st.OriginalURL, &st.Clicks, &st.CreatedAt, &st.ExpiresAt)

	if err != nil {
		return Stats{}, false
	}

	return st, true
}