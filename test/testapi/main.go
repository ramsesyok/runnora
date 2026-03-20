package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	_ "github.com/sijms/go-ora/v2"
)

// --- Models ---

type User struct {
	ID        int64      `json:"id"`
	Name      string     `json:"name"`
	Email     string     `json:"email"`
	Age       *int       `json:"age"`
	CreatedAt time.Time  `json:"created_at"`
}

type CreateUserRequest struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Age   *int   `json:"age"`
}

type UpdateUserRequest struct {
	Name  *string `json:"name"`
	Email *string `json:"email"`
	Age   *int    `json:"age"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

// --- DB ---

var db *sql.DB

func openDB() (*sql.DB, error) {
	dsn := os.Getenv("DB_DSN")
	if dsn == "" {
		dsn = "oracle://testuser:TestPass1!@localhost:1521/FREEPDB1"
	}
	d, err := sql.Open("oracle", dsn)
	if err != nil {
		return nil, err
	}
	d.SetMaxOpenConns(5)
	d.SetMaxIdleConns(2)
	d.SetConnMaxLifetime(5 * time.Minute)
	return d, nil
}

// --- Handlers ---

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	dbStatus := "ok"
	if err := db.PingContext(r.Context()); err != nil {
		dbStatus = "ng: " + err.Error()
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
		"db":     dbStatus,
	})
}

func handleListUsers(w http.ResponseWriter, r *http.Request) {
	rows, err := db.QueryContext(r.Context(),
		`SELECT id, name, email, age, created_at FROM testuser.users ORDER BY id`)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	defer rows.Close()

	users := []User{}
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Name, &u.Email, &u.Age, &u.CreatedAt); err != nil {
			writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
			return
		}
		users = append(users, u)
	}
	writeJSON(w, http.StatusOK, users)
}

func handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var req CreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid JSON: " + err.Error()})
		return
	}
	if req.Name == "" || req.Email == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "name and email are required"})
		return
	}

	// INSERT (RETURNING INTO は go-ora との相性が悪いため SELECT で取得)
	_, err := db.ExecContext(r.Context(),
		`INSERT INTO testuser.users (name, email, age) VALUES (:1, :2, :3)`,
		req.Name, req.Email, req.Age)
	if err != nil {
		// ORA-00001: unique constraint violation
		if isUniqueViolation(err) {
			writeJSON(w, http.StatusConflict, ErrorResponse{Error: "email already exists"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	var u User
	err = db.QueryRowContext(r.Context(),
		`SELECT id, name, email, age, created_at FROM testuser.users WHERE email = :1`,
		req.Email).Scan(&u.ID, &u.Name, &u.Email, &u.Age, &u.CreatedAt)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, u)
}

func handleGetUser(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid id"})
		return
	}

	var u User
	err = db.QueryRowContext(r.Context(),
		`SELECT id, name, email, age, created_at FROM testuser.users WHERE id = :1`, id).
		Scan(&u.ID, &u.Name, &u.Email, &u.Age, &u.CreatedAt)
	if err == sql.ErrNoRows {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "user not found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, u)
}

func handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid id"})
		return
	}

	var req UpdateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid JSON: " + err.Error()})
		return
	}

	// 存在確認
	var exists int
	if err := db.QueryRowContext(r.Context(),
		`SELECT COUNT(*) FROM testuser.users WHERE id = :1`, id).Scan(&exists); err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	if exists == 0 {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "user not found"})
		return
	}

	// 動的 UPDATE
	if req.Name != nil {
		if _, err := db.ExecContext(r.Context(),
			`UPDATE testuser.users SET name = :1 WHERE id = :2`, *req.Name, id); err != nil {
			writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
			return
		}
	}
	if req.Email != nil {
		if _, err := db.ExecContext(r.Context(),
			`UPDATE testuser.users SET email = :1 WHERE id = :2`, *req.Email, id); err != nil {
			writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
			return
		}
	}
	if req.Age != nil {
		if _, err := db.ExecContext(r.Context(),
			`UPDATE testuser.users SET age = :1 WHERE id = :2`, *req.Age, id); err != nil {
			writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
			return
		}
	}

	var u User
	if err := db.QueryRowContext(r.Context(),
		`SELECT id, name, email, age, created_at FROM testuser.users WHERE id = :1`, id).
		Scan(&u.ID, &u.Name, &u.Email, &u.Age, &u.CreatedAt); err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, u)
}

func handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid id"})
		return
	}

	res, err := db.ExecContext(r.Context(),
		`DELETE FROM testuser.users WHERE id = :1`, id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "user not found"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	// ORA-00001
	return contains(err.Error(), "ORA-00001") || contains(err.Error(), "unique constraint")
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// --- Main ---

func main() {
	var err error
	db, err = openDB()
	if err != nil {
		log.Fatalf("DB open: %v", err)
	}
	defer db.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", handleHealth)
	mux.HandleFunc("GET /users", handleListUsers)
	mux.HandleFunc("POST /users", handleCreateUser)
	mux.HandleFunc("GET /users/{id}", handleGetUser)
	mux.HandleFunc("PUT /users/{id}", handleUpdateUser)
	mux.HandleFunc("DELETE /users/{id}", handleDeleteUser)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}
	addr := fmt.Sprintf(":%s", port)
	log.Printf("testapi listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server: %v", err)
	}
}
