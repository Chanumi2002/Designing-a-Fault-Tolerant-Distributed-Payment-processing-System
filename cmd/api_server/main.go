package main

import (
	"log"
	"net/http"

	"distributed_payment_system/internal/api"
)

func main() {
	mux := http.NewServeMux()

	handler := api.NewHandler()
	api.RegisterRoutes(mux, handler)

	fileServer := http.FileServer(http.Dir("web"))
	mux.Handle("/", fileServer)

	addr := ":8080"
	log.Printf("API server running at http://localhost%s\n", addr)
	log.Printf("Open http://localhost%s/index.html\n", addr)

	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}