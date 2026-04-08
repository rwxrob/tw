package serve

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const belaboxRemoteURL = "wss://remote.belabox.net/ws/remote"

type belaboxLiveState struct {
	mu     sync.Mutex
	isLive bool
	ch     chan bool
}

func newBelaboxLiveState() *belaboxLiveState {
	return &belaboxLiveState{ch: make(chan bool, 4)}
}

func (bs *belaboxLiveState) set(live bool) {
	bs.mu.Lock()
	prev := bs.isLive
	bs.isLive = live
	bs.mu.Unlock()
	if live != prev {
		log.Printf("belabox: live=%v", live)
		select {
		case bs.ch <- live:
		default:
		}
	}
}

func (bs *belaboxLiveState) get() bool {
	bs.mu.Lock()
	defer bs.mu.Unlock()
	return bs.isLive
}

func startBelabox(cfg *config, bs *belaboxLiveState) {
	key := belaboxLoadKey(cfg.belaboxRemoteKeyFile)
	if key == "" {
		log.Printf("belabox: no key at %s; Belabox Cloud integration disabled", cfg.belaboxRemoteKeyFile)
		return
	}
	for {
		if err := belaboxConnect(key, bs); err != nil {
			log.Printf("belabox: %v; reconnecting in 5s", err)
		}
		bs.set(false)
		time.Sleep(5 * time.Second)
	}
}

func belaboxLoadKey(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimRight(string(data), "\r\n")
}

func belaboxConnect(key string, bs *belaboxLiveState) error {
	ws, _, err := websocket.DefaultDialer.Dial(belaboxRemoteURL, nil)
	if err != nil {
		return err
	}
	defer ws.Close()

	done := make(chan struct{})
	defer close(done)
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				ws.WriteMessage(websocket.PingMessage, nil) //nolint
			}
		}
	}()

	authMsg, _ := json.Marshal(map[string]any{
		"remote": map[string]any{
			"auth/key": map[string]any{
				"key":     key,
				"version": 6,
			},
		},
	})
	if err := ws.WriteMessage(websocket.TextMessage, authMsg); err != nil {
		return err
	}
	log.Printf("belabox: connecting to remote cloud")

	for {
		_, raw, err := ws.ReadMessage()
		if err != nil {
			return err
		}
		if len(raw) == 0 {
			continue
		}

		var msg map[string]json.RawMessage
		if err := json.Unmarshal(raw, &msg); err != nil {
			continue
		}

		if remoteRaw, ok := msg["remote"]; ok {
			var remote map[string]json.RawMessage
			if err := json.Unmarshal(remoteRaw, &remote); err == nil {
				if authKeyRaw, ok := remote["auth/key"]; ok {
					var authed bool
					json.Unmarshal(authKeyRaw, &authed) //nolint
					if authed {
						log.Printf("belabox: authenticated")
					} else {
						return fmt.Errorf("auth failed; check BELABOX_REMOTE_KEY")
					}
				}
			}
		}

		if netifRaw, ok := msg["netif"]; ok {
			var netifs map[string]struct {
				Tp float64 `json:"tp"`
			}
			if err := json.Unmarshal(netifRaw, &netifs); err != nil {
				continue
			}
			var totalTp float64
			for _, iface := range netifs {
				totalTp += iface.Tp
			}
			totalKbps := totalTp * 8 / 1024
			log.Printf("belabox: netif total=%.0f kbps", totalKbps)
			bs.set(totalKbps > 100)
		}

		if errRaw, ok := msg["error"]; ok {
			var e struct{ Msg string `json:"msg"` }
			json.Unmarshal(errRaw, &e) //nolint
			log.Printf("belabox: error from cloud: %s", e.Msg)
		}
	}
}
