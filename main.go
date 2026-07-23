package main

import (
	"log"
	"net/http"

	"shorten-link/internal/handler"
	"shorten-link/internal/store"
)


func main() {
	st, err := store.New("shorten.db")
	if err != nil {
		log.Fatal(err)
	}

	h := handler.New(st, "http://localhost:8080")

	addr := ":8080"
	log.Printf("server listening on http://localhost%s", addr)

	if err := http.ListenAndServe(addr, h.Routes()); err != nil {
		log.Fatal(err)
	}
}
