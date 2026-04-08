package serve

import (
	"bytes"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/rwxrob/tw/obs"
)

func startHTTP(cfg *config) {
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		topic := readLine1(cfg.topicsFile)
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		_, _ = w.Write([]byte(topic + "\n"))
	})

	mux.HandleFunc("/overlay", func(w http.ResponseWriter, r *http.Request) {
		tmplBytes := overlayTemplate()
		tmpl, err := template.New("overlay").Parse(string(tmplBytes))
		if err != nil {
			http.Error(w, "template error: "+err.Error(), 500)
			return
		}
		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, map[string]string{
			"Topic": readLine1(cfg.topicsFile),
		}); err != nil {
			http.Error(w, "render error: "+err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(buf.Bytes())
	})

	mux.HandleFunc("/logo.jpg", func(w http.ResponseWriter, r *http.Request) {
		logoPath := filepath.Join(os.Getenv("HOME"), ".config", "tw", "logo.jpg")
		if _, err := os.Stat(logoPath); err == nil {
			http.ServeFile(w, r, logoPath)
			return
		}
		w.Header().Set("Content-Type", "image/svg+xml")
		_, _ = w.Write([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="18" height="18"><circle cx="9" cy="9" r="9" fill="#9146ff"/></svg>`))
	})

	addr := ":" + cfg.port
	log.Printf("http: listening on %s (overlay at /overlay)", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Printf("http: server error: %v", err)
	}
}

// overlayTemplate returns ~/.config/tw/overlay.html if present, else the embedded default.
func overlayTemplate() []byte {
	custom := filepath.Join(os.Getenv("HOME"), ".config", "tw", "overlay.html")
	if data, err := os.ReadFile(custom); err == nil {
		return data
	}
	return obs.Overlay
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
