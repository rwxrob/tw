package serve

import (
	_ "embed"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"path/filepath"
	"sync"

	"github.com/gorilla/websocket"
	_ "modernc.org/sqlite"
)

//go:embed clips.html
var clipsHTML []byte

type clipsMsg struct {
	Src   string `json:"src,omitempty"`
	Event string `json:"event,omitempty"`
}

type clipsHub struct {
	mu      sync.Mutex
	clients map[*websocket.Conn]struct{}
}

var clipsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func registerClipsRoutes(cfg *config, mux *http.ServeMux) {
	hub := &clipsHub{clients: make(map[*websocket.Conn]struct{})}

	mux.HandleFunc("/clips", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(clipsHTML)
	})

	mux.HandleFunc("/clips/ws", func(w http.ResponseWriter, r *http.Request) {
		hub.serveWS(cfg, w, r)
	})

	mux.Handle("/clip/", http.StripPrefix("/clip/", http.FileServer(http.Dir(cfg.clipsDir))))
}

func (h *clipsHub) serveWS(cfg *config, w http.ResponseWriter, r *http.Request) {
	c, err := clipsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("clips: ws upgrade: %v", err)
		return
	}
	defer func() {
		h.mu.Lock()
		delete(h.clients, c)
		h.mu.Unlock()
		c.Close()
	}()

	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()

	if slug := randomClipSlug(cfg); slug != "" {
		sendClipMsg(c, slug)
	}

	for {
		_, raw, err := c.ReadMessage()
		if err != nil {
			return
		}
		var msg clipsMsg
		if err := json.Unmarshal(raw, &msg); err != nil {
			continue
		}
		if msg.Event == "ended" {
			if slug := randomClipSlug(cfg); slug != "" {
				sendClipMsg(c, slug)
			}
		}
	}
}

func sendClipMsg(c *websocket.Conn, slug string) {
	b, _ := json.Marshal(clipsMsg{Src: "/clip/" + slug + ".mp4"})
	_ = c.WriteMessage(websocket.TextMessage, b)
}

func randomClipSlug(cfg *config) string {
	dbPath := filepath.Join(cfg.clipsDir, "clips.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		log.Printf("clips: open db: %v", err)
		return ""
	}
	defer db.Close()

	var slug string
	if err := db.QueryRow(
		`SELECT slug FROM clips WHERE downloaded=1 AND slug!='' ORDER BY RANDOM()*weight DESC LIMIT 1`,
	).Scan(&slug); err != nil {
		log.Printf("clips: random clip: %v", err)
		return ""
	}
	return slug
}
