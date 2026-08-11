package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"

	_ "github.com/lib/pq"
)

var db *sql.DB

func main() {
	connStr := os.Getenv("DATABASE_URL")
	if connStr == "" {
		connStr = "postgres://postgres:postgres@db:5432/aut?sslmode=disable"
	}

	var err error
	for i := 0; i < 10; i++ {
		db, err = sql.Open("postgres", connStr)
		if err == nil {
			if pingErr := db.Ping(); pingErr == nil {
				break
			}
		}
		log.Println("waiting for database...")
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		log.Fatal("could not connect to database:", err)
	}

	db.Exec("CREATE TABLE IF NOT EXISTS items (id SERIAL PRIMARY KEY, name TEXT)")

	mux := http.NewServeMux()
	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("/items", itemsHandler)

	log.Println("AUT api listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	if err := db.Ping(); err != nil {
		http.Error(w, "database unreachable: "+err.Error(), http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

type Item struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func itemsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		rows, err := db.Query("SELECT id, name FROM items")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer rows.Close()
		var items []Item
		for rows.Next() {
			var it Item
			rows.Scan(&it.ID, &it.Name)
			items = append(items, it)
		}
		json.NewEncoder(w).Encode(items)
	case http.MethodPost:
		var item Item
		if err := json.NewDecoder(r.Body).Decode(&item); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		err := db.QueryRow("INSERT INTO items (name) VALUES ($1) RETURNING id", item.Name).Scan(&item.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(item)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
