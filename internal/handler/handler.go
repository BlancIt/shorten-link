package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"shorten-link/internal/store"
	"shorten-link/internal/validate"
)

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

// Bundles the dependencies for the handlers
type Handler struct {
	store   *store.Store
	baseURL string
}

// Creates a Handler backed by the given store.
func New(st *store.Store, baseURL string) *Handler {
	return &Handler{store: st, baseURL: baseURL}
}

// Returns a mux with all routes registered.
func (h *Handler) Routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /shorten", h.Shorten)
	mux.HandleFunc("GET /{code}", h.Redirect)
	mux.HandleFunc("GET /stats/{code}", h.Stats)
	return mux
}

// Handles POST /shorten.
func (h *Handler) Shorten(w http.ResponseWriter, r *http.Request) {
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

	if !validate.URL(req.URL) {
		http.Error(w, "invalid url: must be a valid http or https URL", http.StatusBadRequest)
		return
	}

	if req.ExpiresIn > 0 {
		t := time.Now().Add(time.Duration(req.ExpiresIn) * time.Second)
		expiresAt = &t
	}

	if req.Alias != "" {
		if !validate.Alias(req.Alias) {
			http.Error(w, "invalid alias: 5-20 chars, letters/digits/-/_ only", http.StatusBadRequest)
			return
		}

		if err := h.store.SaveWithCode(req.Alias, req.URL, expiresAt); err != nil {
			http.Error(w, "alias already taken", http.StatusConflict) // 409
			return
		}
		code = req.Alias
	} else {
		var err error
		code, err = h.store.Save(req.URL, expiresAt)

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
func (h *Handler) Redirect(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	originalURL, ok := h.store.Get(code)

	if !ok {
		http.NotFound(w, r)
		return
	}

	h.store.IncrementClicks(code)  
	http.Redirect(w, r, originalURL, http.StatusFound)
}

// Handles GET /stats/{code}.
func (h *Handler) Stats(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	st, ok := h.store.GetStats(code)

	if !ok {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(st)
}