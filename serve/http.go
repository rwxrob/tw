package serve

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/rwxrob/tw/obs"
)

// sseBroker fans out topic changes to all connected SSE clients.
type sseBroker struct {
	mu      sync.Mutex
	clients map[chan string]struct{}
}

func newSSEBroker() *sseBroker {
	return &sseBroker{clients: make(map[chan string]struct{})}
}

func (b *sseBroker) subscribe() chan string {
	ch := make(chan string, 4)
	b.mu.Lock()
	b.clients[ch] = struct{}{}
	b.mu.Unlock()
	return ch
}

func (b *sseBroker) unsubscribe(ch chan string) {
	b.mu.Lock()
	delete(b.clients, ch)
	b.mu.Unlock()
}

func (b *sseBroker) publish(topic string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.clients {
		select {
		case ch <- topic:
		default:
		}
	}
}

// watchTopicFile polls the topics file and publishes to the broker whenever line 1 changes.
func watchTopicFile(cfg *config, broker *sseBroker) {
	var last string
	for {
		current := readLine1(cfg.topicsFile)
		if current != last {
			last = current
			broker.publish(current)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func startHTTP(cfg *config) {
	broker := newSSEBroker()
	go watchTopicFile(cfg, broker)

	mux := http.NewServeMux()

	// GET / — plain text current topic (kept for backward compat)
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

	// GET /overlay — full HTML overlay page
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

	// GET /events — SSE stream of topic changes
	mux.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		fl, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Connection", "keep-alive")

		ch := broker.subscribe()
		defer broker.unsubscribe(ch)

		// send current topic immediately on connect
		_, _ = fmt.Fprintf(w, "data: %s\n\n", readLine1(cfg.topicsFile))
		fl.Flush()

		for {
			select {
			case topic := <-ch:
				_, _ = fmt.Fprintf(w, "data: %s\n\n", topic)
				fl.Flush()
			case <-r.Context().Done():
				return
			}
		}
	})

	// GET /logo.jpg — serve ~/.config/tw/logo.jpg or SVG placeholder
	mux.HandleFunc("/logo.jpg", func(w http.ResponseWriter, r *http.Request) {
		logoPath := filepath.Join(os.Getenv("HOME"), ".config", "tw", "logo.jpg")
		if _, err := os.Stat(logoPath); err == nil {
			http.ServeFile(w, r, logoPath)
			return
		}
		w.Header().Set("Content-Type", "image/svg+xml")
		_, _ = w.Write([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="18" height="18"><circle cx="9" cy="9" r="9" fill="#9146ff"/></svg>`))
	})

	registerClipsRoutes(cfg, mux)

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
