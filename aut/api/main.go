package main

import (
"encoding/json"
"log"
"net/http"
"sync"
)

type Item struct {
ID   int    `json:"id"`
Name string `json:"name"`
}

var (
mu     sync.Mutex
items  = []Item{{ID: 1, Name: "seed item"}}
nextID = 2
)

func main() {
mux := http.NewServeMux()
mux.HandleFunc("/health", healthHandler)
mux.HandleFunc("/items", itemsHandler)

log.Println("AUT api listening on :8080")
log.Fatal(http.ListenAndServe(":8080", mux))
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
w.WriteHeader(http.StatusOK)
w.Write([]byte("ok"))
}

func itemsHandler(w http.ResponseWriter, r *http.Request) {
mu.Lock()
defer mu.Unlock()

switch r.Method {
case http.MethodGet:
json.NewEncoder(w).Encode(items)
case http.MethodPost:
var item Item
if err := json.NewDecoder(r.Body).Decode(&item); err != nil {
http.Error(w, err.Error(), http.StatusBadRequest)
return
}
item.ID = nextID
nextID++
items = append(items, item)
json.NewEncoder(w).Encode(item)
default:
http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}
}
