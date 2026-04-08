package serve

import (
	"log"
	"net/http"
	"os"
	"strings"
)

func startHTTP(cfg *config) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		topic := readLine1(cfg.topicsFile)
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(topic + "\n"))
	})

	addr := ":" + cfg.port
	log.Printf("http: listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Printf("http: server error: %v", err)
	}
}

func readLine1(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	lines := strings.SplitN(string(data), "\n", 2)
	if len(lines) == 0 {
		return ""
	}
	return strings.TrimRight(lines[0], "\r")
}
