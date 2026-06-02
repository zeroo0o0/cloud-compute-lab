package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func getenv(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func appendLoginLog(logDir, player string) error {
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return err
	}
	logPath := filepath.Join(logDir, "players.log")
	line := fmt.Sprintf("%s: %s logged in\n", time.Now().Format(time.RFC3339), player)
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(line)
	return err
}

func main() {
	configPath := getenv("CONFIG_PATH", "/etc/game/config.yaml")
	logDir := getenv("LOG_DIR", "/app/data")
	addr := getenv("DEMO_ADDR", "0.0.0.0:8080")

	content, err := os.ReadFile(configPath)
	if err != nil {
		log.Fatalf("missing config: %s: %v", configPath, err)
	}
	log.Printf("loaded config from %s", configPath)
	log.Printf("config preview: %s", strings.TrimSpace(string(content)))

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Server Started")
	})
	mux.HandleFunc("/login", func(w http.ResponseWriter, r *http.Request) {
		player := strings.TrimSpace(r.URL.Query().Get("player"))
		if player == "" {
			player = "guest"
		}
		if err := appendLoginLog(logDir, player); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		fmt.Fprintf(w, "login ok: %s\n", player)
	})

	log.Printf("Server Started")
	log.Printf("listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server exit: %v", err)
	}
}
