package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync/atomic"
)

type api_config struct {
	file_server_hits atomic.Uint32
}

func (config *api_config) middle_ware_increment(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		config.file_server_hits.Add(1)
		next.ServeHTTP(w, r)
	})
}

func (config *api_config) metrics_handler(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Content-Type", "text/html")
	writer.WriteHeader(http.StatusOK)
	fmt.Fprintf(writer, "<html> <body> <h1>Welcome, Chirpy Admin</h1> <p>Chirpy has been visited %d times!</p> </body> </html>", config.file_server_hits.Load())
}

func (config *api_config) reset_handler(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
	writer.WriteHeader(http.StatusOK)
	writer.Write([]byte("Reset visit counter to 0"))
	config.file_server_hits.Swap(0)
}

func respond_with_error(writer http.ResponseWriter, code int, msg string, err error) {
	if err != nil {
		log.Println(err)
	}

	type response_error struct {
		Error string `json:"error"`
	}
	respond_with_json(writer, code, response_error{Error: msg})
}

func respond_with_json(writer http.ResponseWriter, code int, payload interface{}) {
	writer.Header().Set("Content-Type", "application/json")
	response_bytes, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Error marshaling JSON: %s", err)
		writer.WriteHeader(500)
		return
	}
	writer.WriteHeader(code)
	writer.Write(response_bytes)
}

func main() {
	api_config := api_config{}
	handler := http.NewServeMux()
	handler.Handle("/app/", api_config.middle_ware_increment(http.StripPrefix("/app", http.FileServer(http.Dir(".")))))
	handler.HandleFunc("GET /api/healthz", healthHandler)
	handler.HandleFunc("POST /api/validate_chirp", validate_handler)
	handler.HandleFunc("GET /admin/metrics", api_config.metrics_handler)
	handler.HandleFunc("POST /admin/reset", api_config.reset_handler)
	server := &http.Server{
		Addr:    ":8080",
		Handler: handler,
	}
	log.Printf("serving on port: %s", "8080")
	log.Fatal(server.ListenAndServe())
}

// validateing that the body isn't longer than 140 characters
func validate_handler(writer http.ResponseWriter, request *http.Request) {
	type chirp struct {
		Body string `json:"body"`
	}

	type valid_chirp_response struct {
		Valid bool `json:"valid"`
	}

	decoder := json.NewDecoder(request.Body)
	chirp_instance := chirp{}
	err := decoder.Decode(&chirp_instance)
	if err != nil {
		respond_with_error(writer, http.StatusInternalServerError, "Couldn't decode chirp", err)
		return
	}
	if len(chirp_instance.Body) > 140 {
		respond_with_error(writer, http.StatusBadRequest, "Chirp is too long", nil)
		return
	}
	respond_with_json(writer, http.StatusOK, valid_chirp_response{Valid: true})
}

func healthHandler(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
	writer.WriteHeader(http.StatusOK)
	writer.Write([]byte("OK"))
}
