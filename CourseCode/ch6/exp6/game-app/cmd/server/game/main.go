package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
)

type MoveResponse struct {
	OK      bool   `json:"ok"`
	Service string `json:"service"`
	Message string `json:"message"`
}

func writeJSON(w http.ResponseWriter, code int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("[game] failed to write response: %v", err)
	}
}

func main() {
	addr := os.Getenv("GAME_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8081"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/move", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[game] %s /move from %s", r.Method, r.RemoteAddr)
		writeJSON(w, http.StatusOK, MoveResponse{
			OK:      true,
			Service: "game",
			Message: "move accepted",
		})
	})
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})

	log.Printf("[game] started on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("[game] server exit: %v", err)
	}
}
