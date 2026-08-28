package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/AstorDG/chirpy.git/internal/database"
	"github.com/google/uuid"
	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

type api_config struct {
	file_server_hits atomic.Uint32
	db               *database.Queries
}

type user struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Email     string    `json:"email"`
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

func (config *api_config) new_user_handler(writer http.ResponseWriter, request *http.Request) {
	type email struct {
		Email string `json:"email"`
	}

	decoder := json.NewDecoder(request.Body)
	new_user_email := email{}
	err := decoder.Decode(&new_user_email)
	if err != nil {
		respond_with_error(writer, 500, "Couldn't decode email", err)
	}
	new_user_database, err := config.db.CreateUser(request.Context(), new_user_email.Email)

	new_user_native := user{
		ID:        new_user_database.ID,
		CreatedAt: new_user_database.CreatedAt,
		UpdatedAt: new_user_database.UpdatedAt,
		Email:     new_user_database.Email,
	}

	respond_with_json(writer, 201, new_user_native)
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
	godotenv.Load()
	dbURL := os.Getenv("DB_URL")
	if dbURL == "" {
		log.Fatal("DB_URL must be set")
	}
	db_connection, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("Error opening database %s", err)
	}
	db_queries := database.New(db_connection)
	api_config := api_config{
		file_server_hits: atomic.Uint32{},
		db:               db_queries,
	}
	handler := http.NewServeMux()
	handler.Handle("/app/", api_config.middle_ware_increment(http.StripPrefix("/app", http.FileServer(http.Dir(".")))))
	handler.HandleFunc("GET /api/healthz", healthHandler)
	handler.HandleFunc("POST /api/validate_chirp", validate_handler)
	handler.HandleFunc("POST /api/users", api_config.new_user_handler)
	handler.HandleFunc("GET /admin/metrics", api_config.metrics_handler)
	handler.HandleFunc("POST /admin/reset", api_config.reset_handler)
	server := &http.Server{
		Addr:    ":8080",
		Handler: handler,
	}
	log.Printf("serving on port: %s", "8080")
	log.Fatal(server.ListenAndServe())
}

func censor_profane_words(body string) string {
	body_lower_case_split := strings.Split(body, " ")
	for index := range body_lower_case_split {
		switch strings.ToLower(body_lower_case_split[index]) {
		case "kerfuffle":
			body_lower_case_split[index] = "****"
		case "sharbert":
			body_lower_case_split[index] = "****"
		case "fornax":
			body_lower_case_split[index] = "****"
		default:
			continue
		}
	}
	return strings.Join(body_lower_case_split, " ")
}

// validateing that the body isn't longer than 140 characters
func validate_handler(writer http.ResponseWriter, request *http.Request) {
	type chirp struct {
		Body string `json:"body"`
	}

	type cleaned_chirp struct {
		Cleaned_Body string `json:"cleaned_body"`
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
	cleaned_chirp_instance := censor_profane_words(chirp_instance.Body)

	respond_with_json(writer, http.StatusOK, cleaned_chirp{Cleaned_Body: cleaned_chirp_instance})
}

func healthHandler(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
	writer.WriteHeader(http.StatusOK)
	writer.Write([]byte("OK"))
}
