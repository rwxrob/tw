package serve

import (
	_ "embed"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"sync"

	"github.com/gorilla/websocket"
	_ "modernc.org/sqlite"
)

//go:embed clips.html
var clipsHTML []byte

//go:embed retrotv
var retrotvHTML []byte

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

	mux.HandleFunc("/clips/retrotv", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(retrotvHTML)
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

	if id := randomClipID(cfg); id != 0 {
		sendClipMsg(c, id)
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
			if id := randomClipID(cfg); id != 0 {
				sendClipMsg(c, id)
			}
		}
	}
}

func sendClipMsg(c *websocket.Conn, id int) {
	b, _ := json.Marshal(clipsMsg{Src: fmt.Sprintf("/clip/%d.mp4", id)})
	_ = c.WriteMessage(websocket.TextMessage, b)
}

func randomClipID(cfg *config) int {
	dbPath := filepath.Join(cfg.clipsDir, "clips.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		log.Printf("clips: open db: %v", err)
		return 0
	}
	defer db.Close()

	var id int
	if err := db.QueryRow(
		`SELECT id FROM clips WHERE downloaded=1 ORDER BY RANDOM()*weight DESC LIMIT 1`,
	).Scan(&id); err != nil {
		log.Printf("clips: random clip: %v", err)
		return 0
	}
	return id
}
