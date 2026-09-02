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
	platform         string
	secret           string
}

type user struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Email     string    `json:"email"`
	Password  string    `json:"-"`
	Token     string    `json:"token"`
}

type chirp struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Body      string    `json:"body"`
	UserId    uuid.UUID `json:"user_id"`
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
	if config.platform != "dev" {
		writer.WriteHeader(http.StatusForbidden)
		writer.Write([]byte("Reset is only allowed in Dev environments"))
		return
	}
	config.file_server_hits.Swap(0)
	err := config.db.ResetTable(request.Context())
	if err != nil {
		writer.WriteHeader(http.StatusInternalServerError)
		writer.Write([]byte("Table couldn't be reset" + err.Error()))
		return
	}
	writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
	writer.WriteHeader(http.StatusOK)
	writer.Write([]byte("All users deleted and hits set to 0"))
}

func (config *api_config) new_user_handler(writer http.ResponseWriter, request *http.Request) {
	type new_user_input struct {
		Password string `json:"password"`
		Email    string `json:"email"`
	}

	decoder := json.NewDecoder(request.Body)
	new_user := new_user_input{}
	err := decoder.Decode(&new_user)
	if err != nil {
		respond_with_error(writer, http.StatusInternalServerError, "Couldn't decode email or password", err)
		return
	}

	hashed_password, err := hash_password(new_user.Password)
	if err != nil {
		respond_with_error(writer, http.StatusInternalServerError, "Coulnd't hash password", err)
	}

	new_user_database, err := config.db.CreateUser(request.Context(), database.CreateUserParams{Email: new_user.Email, HashedPassword: hashed_password})
	if err != nil {
		respond_with_error(writer, http.StatusInternalServerError, "Couldn't Create user", err)
		return
	}

	new_user_native := user{
		ID:        new_user_database.ID,
		CreatedAt: new_user_database.CreatedAt,
		UpdatedAt: new_user_database.UpdatedAt,
		Email:     new_user_database.Email,
	}

	respond_with_json(writer, 201, new_user_native)
}

// validateing a chirp and adding it to the database
func (config *api_config) new_chirp_handler(writer http.ResponseWriter, request *http.Request) {
	type new_chirp_native struct {
		Body string `json:"body"`
	}

	jwt_token, err := get_bearer_token(request.Header)
	if err != nil {
		respond_with_error(writer, http.StatusBadRequest, "header formatted wrong", err)
		return
	}

	user_id, err := validate_jwt(jwt_token, config.secret)
	if err != nil {
		respond_with_error(writer, 401, "Unauthorized", err)
		return
	}

	decoder := json.NewDecoder(request.Body)
	chirp_instance := new_chirp_native{}
	err = decoder.Decode(&chirp_instance)
	if err != nil {
		respond_with_error(writer, http.StatusInternalServerError, "Couldn't decode chirp", err)
		return
	}
	if len(chirp_instance.Body) > 140 {
		respond_with_error(writer, http.StatusBadRequest, "Chirp is too long", nil)
		return
	}
	chirp_instance.Body = censor_profane_words(chirp_instance.Body)
	new_chirp_database, err := config.db.CreateChirp(request.Context(), database.CreateChirpParams{Body: chirp_instance.Body, UserID: user_id})
	if err != nil {
		respond_with_error(writer, http.StatusInternalServerError, "Couldn't add chirp to database", err)
		return
	}
	chirp_native := chirp{
		ID:        new_chirp_database.ID,
		CreatedAt: new_chirp_database.CreatedAt,
		UpdatedAt: new_chirp_database.UpdatedAt,
		Body:      new_chirp_database.Body,
		UserId:    new_chirp_database.UserID,
	}

	respond_with_json(writer, 201, chirp_native)
}

func (config *api_config) get_all_chirps_handler(writer http.ResponseWriter, request *http.Request) {

	all_users, err := config.db.GetChirps(request.Context())
	if err != nil {
		respond_with_error(writer, http.StatusInternalServerError, "Couldn't get chirps from the server", err)
	}
	native_chirps := make([]chirp, 0, len(all_users))

	for _, db_chirp := range all_users {
		native_chirps = append(native_chirps, chirp{
			ID:        db_chirp.ID,
			CreatedAt: db_chirp.CreatedAt,
			UpdatedAt: db_chirp.UpdatedAt,
			Body:      db_chirp.Body,
			UserId:    db_chirp.UserID,
		})
	}
	respond_with_json(writer, http.StatusOK, native_chirps)
}

func (config *api_config) get_chirp_handler(writer http.ResponseWriter, request *http.Request) {
	id, err := uuid.Parse(request.PathValue("chirpID"))
	if err != nil {
		respond_with_error(writer, 500, "Invalid UUID", err)
		return
	}

	recieved_chirp, err := config.db.GetChirp(request.Context(), id)
	if err != nil {
		respond_with_error(writer, 404, "chirp not found", err)
		return
	}
	respond_with_json(writer, http.StatusOK, chirp{
		ID:        recieved_chirp.ID,
		CreatedAt: recieved_chirp.CreatedAt,
		UpdatedAt: recieved_chirp.UpdatedAt,
		Body:      recieved_chirp.Body,
		UserId:    recieved_chirp.UserID,
	})
}

func (config *api_config) login_handler(writer http.ResponseWriter, request *http.Request) {
	type client_user_info struct {
		Password string `json:"password"`
		Email    string `json:"email"`
	}

	decoder := json.NewDecoder(request.Body)
	login_user := client_user_info{}
	err := decoder.Decode(&login_user)
	if err != nil {
		respond_with_error(writer, http.StatusBadRequest, "Not valid user information", err)
		return
	}

	user_info, err := config.db.GetUser(request.Context(), login_user.Email)
	if err != nil {
		respond_with_error(writer, 401, "Incorrect email or password", err)
		return
	}
	passwords_match, err := check_password_hash(login_user.Password, user_info.HashedPassword)
	if err != nil || !passwords_match {
		respond_with_error(writer, 401, "Incorrect email or password", err)
		return
	}

	access_token, err := make_jwt(user_info.ID, config.secret, time.Duration(int(time.Hour)))
	if err != nil {
		respond_with_error(writer, http.StatusInternalServerError, "Couldn't make jwt", err)
		return
	}
	refresh_token := make_refresh_token()

	_, err = config.db.CreateRefreshToken(request.Context(), database.CreateRefreshTokenParams{
		Token:     refresh_token,
		UserID:    user_info.ID,
		ExpiresAt: time.Now().Add(time.Hour * 24 * 60)})
	if err != nil {
		respond_with_error(writer, http.StatusInternalServerError, "Database coulnd't create a refresh token", err)
		return
	}

	type user_token struct {
		ID            uuid.UUID `json:"id"`
		CreatedAt     time.Time `json:"created_at"`
		UpdatedAt     time.Time `json:"updated_at"`
		Email         string    `json:"email"`
		Token         string    `json:"token"`
		Refresh_token string    `json:"refresh_token"`
	}

	user_no_pass := user_token{
		ID:            user_info.ID,
		CreatedAt:     user_info.CreatedAt,
		UpdatedAt:     user_info.UpdatedAt,
		Email:         user_info.Email,
		Token:         access_token,
		Refresh_token: refresh_token,
	}

	respond_with_json(writer, http.StatusOK, user_no_pass)
}

func (config *api_config) refresh_handler(writer http.ResponseWriter, request *http.Request) {
	refresh_token, err := get_bearer_token(request.Header)
	if err != nil {
		respond_with_error(writer, http.StatusBadRequest, "Refresh token doesn't exist", err)
		return
	}
	db_refresh_token, err := config.db.GetRefreshToken(request.Context(), refresh_token)
	if err != nil {
		respond_with_error(writer, 401, "Invalid Token", err)
		return
	} else if db_refresh_token.RevokedAt.Valid == true {
		respond_with_error(writer, 401, "token revoked", nil)
		return
	} else if time.Now().After(db_refresh_token.ExpiresAt) {
		respond_with_error(writer, 401, "token expired", nil)
		return
	}

	type response struct {
		Token string `json:"token"`
	}

	access_token, err := make_jwt(db_refresh_token.UserID, config.secret, time.Hour)

	if err != nil {
		respond_with_error(writer, http.StatusInternalServerError, "error creating access token", err)
		return
	}

	respond_with_json(writer, 200, response{Token: access_token})
}
func (config *api_config) revoke_handler(writer http.ResponseWriter, request *http.Request) {
	refresh_token, err := get_bearer_token(request.Header)
	if err != nil {
		respond_with_error(writer, http.StatusBadRequest, "Refresh token required", err)
		return
	}

	err = config.db.RevokeToken(request.Context(), database.RevokeTokenParams{RevokedAt: sql.NullTime{Time: time.Now(), Valid: true}, Token: refresh_token})
	if err != nil {
		respond_with_error(writer, http.StatusBadRequest, "Invalid refresh token", err)
		return
	}

	writer.WriteHeader(204)
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
	server_secret := os.Getenv("SECRET")
	if server_secret == "" {
		log.Fatal("Server must have a secret for encryption")
	}

	db_connection, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("Error opening database %s", err)
	}
	db_queries := database.New(db_connection)
	platform := os.Getenv("PLATFORM")
	if platform == "" {
		log.Fatal("PLATFORM must be set")
	}
	api_config := api_config{
		file_server_hits: atomic.Uint32{},
		db:               db_queries,
		platform:         platform,
		secret:           server_secret,
	}
	handler := http.NewServeMux()
	handler.Handle("/app/", api_config.middle_ware_increment(http.StripPrefix("/app", http.FileServer(http.Dir(".")))))
	handler.HandleFunc("GET /api/healthz", healthHandler)
	handler.HandleFunc("GET /api/chirps", api_config.get_all_chirps_handler)
	handler.HandleFunc("GET /api/chirps/{chirpID}", api_config.get_chirp_handler)
	handler.HandleFunc("POST /api/chirps", api_config.new_chirp_handler)
	handler.HandleFunc("POST /api/users", api_config.new_user_handler)
	handler.HandleFunc("POST /api/login", api_config.login_handler)
	handler.HandleFunc("POST /api/refresh", api_config.refresh_handler)
	handler.HandleFunc("POST /api/revoke", api_config.revoke_handler)
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

func healthHandler(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
	writer.WriteHeader(http.StatusOK)
	writer.Write([]byte("OK"))
}
