package serve

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type obsMsg struct {
	Op int             `json:"op"`
	D  json.RawMessage `json:"d"`
}

// obsConn wraps the websocket with a write mutex so multiple goroutines
// can safely send without racing.
type obsConn struct {
	mu sync.Mutex
	ws *websocket.Conn
}

func (c *obsConn) send(reqType string, reqData map[string]any) {
	msg, err := json.Marshal(map[string]any{
		"op": 6,
		"d": map[string]any{
			"requestType": reqType,
			"requestId":   reqType,
			"requestData": reqData,
		},
	})
	if err != nil {
		log.Printf("obs: marshal error: %v", err)
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.ws.WriteMessage(websocket.TextMessage, msg); err != nil {
		log.Printf("obs: write error for %s: %v", reqType, err)
	}
}

type obsState struct {
	mu           sync.Mutex
	currentScene string
	prevScene    string
}

func (s *obsState) scene() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.currentScene
}

func startOBS(cfg *config, bs *belaboxLiveState, state *obsState) {
	for {
		if err := obsConnect(cfg, bs, state); err != nil {
			log.Printf("obs: disconnected: %v; reconnecting in 2s", err)
		}
		time.Sleep(2 * time.Second)
	}
}

func obsConnect(cfg *config, bs *belaboxLiveState, state *obsState) error {
	password := obsLoadPassword(cfg.obsWSPasswordFile)

	ws, _, err := websocket.DefaultDialer.Dial(cfg.obsWSURL, nil)
	if err != nil {
		return err
	}
	c := &obsConn{ws: ws}
	defer ws.Close()

	// Read Hello (op:0)
	_, raw, err := ws.ReadMessage()
	if err != nil {
		return err
	}
	var hello obsMsg
	if err := json.Unmarshal(raw, &hello); err != nil {
		return err
	}

	// Build Identify (op:1)
	type identifyData struct {
		RpcVersion         int    `json:"rpcVersion"`
		Authentication     string `json:"authentication,omitempty"`
		EventSubscriptions int    `json:"eventSubscriptions"`
	}
	// Scenes(4) + Outputs(64) — no longer need MediaInputs(256)
	id := identifyData{RpcVersion: 1, EventSubscriptions: 68}

	var helloD struct {
		Authentication *struct {
			Challenge string `json:"challenge"`
			Salt      string `json:"salt"`
		} `json:"authentication"`
	}
	_ = json.Unmarshal(hello.D, &helloD)
	if helloD.Authentication != nil && password != "" {
		id.Authentication = obsAuthString(password, helloD.Authentication.Salt, helloD.Authentication.Challenge)
	}

	identMsg, _ := json.Marshal(map[string]any{"op": 1, "d": id})
	c.mu.Lock()
	err = ws.WriteMessage(websocket.TextMessage, identMsg)
	c.mu.Unlock()
	if err != nil {
		return err
	}

	// Read Identified (op:2)
	_, raw, err = ws.ReadMessage()
	if err != nil {
		return err
	}
	var identified obsMsg
	if err := json.Unmarshal(raw, &identified); err != nil {
		return err
	}
	if identified.Op != 2 {
		return fmt.Errorf("obs: expected op:2, got op:%d", identified.Op)
	}
	log.Printf("obs: connected to %s", cfg.obsWSURL)

	c.send("GetCurrentProgramScene", map[string]any{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go obsWatchBelabox(ctx, cfg, state, c, bs)

	for {
		_, raw, err := ws.ReadMessage()
		if err != nil {
			return err
		}
		var msg obsMsg
		if err := json.Unmarshal(raw, &msg); err != nil {
			continue
		}
		switch msg.Op {
		case 7:
			obsHandleResponse(cfg, state, c, msg.D)
		case 5:
			obsHandleEvent(cfg, state, c, msg.D)
		}
	}
}

func obsWatchBelabox(ctx context.Context, cfg *config, state *obsState, c *obsConn, bs *belaboxLiveState) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	apply := func(live bool) {
		state.mu.Lock()
		defer state.mu.Unlock()
		if live {
			if state.currentScene == cfg.clipsScene && state.prevScene != "" {
				target := state.prevScene
				state.prevScene = ""
				state.mu.Unlock()
				log.Printf("obs: belabox live; restoring %s", target)
				c.send("SetCurrentProgramScene", map[string]any{"sceneName": target})
				state.mu.Lock()
			}
		} else {
			if state.currentScene != "" && state.currentScene != cfg.clipsScene &&
				strings.HasPrefix(state.currentScene, "IRL") {
				state.prevScene = state.currentScene
				_ = os.MkdirAll(filepath.Dir(cfg.liveSceneFile), 0755)
				_ = os.WriteFile(cfg.liveSceneFile, []byte(state.currentScene+"\n"), 0644)
				state.mu.Unlock()
				log.Printf("obs: belabox down; switching to %s", cfg.clipsScene)
				c.send("SetCurrentProgramScene", map[string]any{"sceneName": cfg.clipsScene})
				state.mu.Lock()
			}
		}
	}

	apply(bs.get())

	for {
		select {
		case <-ctx.Done():
			return
		case live := <-bs.ch:
			apply(live)
		case <-ticker.C:
			apply(bs.get())
		}
	}
}

func obsHandleResponse(cfg *config, state *obsState, c *obsConn, d json.RawMessage) {
	var resp struct {
		RequestType  string          `json:"requestType"`
		ResponseData json.RawMessage `json:"responseData"`
	}
	if err := json.Unmarshal(d, &resp); err != nil {
		return
	}

	switch resp.RequestType {
	case "GetCurrentProgramScene":
		var data struct {
			CurrentProgramSceneName string `json:"currentProgramSceneName"`
		}
		if err := json.Unmarshal(resp.ResponseData, &data); err != nil {
			return
		}
		state.mu.Lock()
		defer state.mu.Unlock()
		state.currentScene = data.CurrentProgramSceneName
		log.Printf("obs: current scene: %s", state.currentScene)
		if state.currentScene == cfg.clipsScene && state.prevScene == "" {
			if raw, err := os.ReadFile(cfg.liveSceneFile); err == nil {
				state.prevScene = strings.TrimSpace(string(raw))
			} else {
				state.prevScene = cfg.liveScene
			}
		}
	}
}

func obsHandleEvent(cfg *config, state *obsState, c *obsConn, d json.RawMessage) {
	var ev struct {
		EventType string          `json:"eventType"`
		EventData json.RawMessage `json:"eventData"`
	}
	if err := json.Unmarshal(d, &ev); err != nil {
		return
	}

	switch ev.EventType {
	case "CurrentProgramSceneChanged":
		var data struct {
			SceneName string `json:"sceneName"`
		}
		if err := json.Unmarshal(ev.EventData, &data); err != nil {
			return
		}
		state.mu.Lock()
		state.currentScene = data.SceneName
		state.mu.Unlock()
		log.Printf("obs: scene changed to %s", data.SceneName)

	case "RecordStateChanged":
		var data struct {
			OutputActive bool   `json:"outputActive"`
			OutputPath   string `json:"outputPath"`
		}
		if err := json.Unmarshal(ev.EventData, &data); err != nil {
			return
		}
		if data.OutputPath != "" {
			obsRenameRecording(cfg, data.OutputPath)
		}
	}
}

func obsRenameRecording(cfg *config, src string) {
	if src == "" || src == "null" {
		return
	}
	if _, err := os.Stat(src); err != nil {
		log.Printf("obs: recording file not found: %s", src)
		return
	}

	topic := readLine1(cfg.topicsFile)
	slug := slugify(topic)
	dir := filepath.Dir(src)
	ext := filepath.Ext(src)
	dest := filepath.Join(dir, slug+ext)

	if dest == src {
		return
	}
	// Handle name collisions
	if _, err := os.Stat(dest); err == nil {
		n := 2
		for {
			candidate := filepath.Join(dir, slug+"-"+strconv.Itoa(n)+ext)
			if _, err := os.Stat(candidate); err != nil {
				dest = candidate
				break
			}
			n++
		}
	}

	if err := os.Rename(src, dest); err != nil {
		log.Printf("obs: rename error: %v", err)
		return
	}
	log.Printf("obs: renamed recording:\n  from: %s\n    to: %s", src, dest)
}

func slugify(s string) string {
	s = stripLeadingEmoji(s)
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	result := b.String()
	for strings.Contains(result, "--") {
		result = strings.ReplaceAll(result, "--", "-")
	}
	result = strings.Trim(result, "-")
	if result == "" {
		result = "untitled-recording"
	}
	if len(result) > 60 {
		result = result[:60]
		result = strings.TrimRight(result, "-")
	}
	return result
}

func stripLeadingEmoji(s string) string {
	if len(s) > 0 && s[0] == ':' {
		end := strings.Index(s[1:], ":")
		if end >= 0 {
			rest := strings.TrimSpace(s[end+2:])
			if rest != "" {
				return rest
			}
		}
	}
	return s
}

func obsLoadPassword(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimRight(string(data), "\r\n")
}

func obsAuthString(password, salt, challenge string) string {
	h := sha256.New()
	h.Write([]byte(password + salt))
	secret := base64.StdEncoding.EncodeToString(h.Sum(nil))

	h2 := sha256.New()
	h2.Write([]byte(secret + challenge))
	return base64.StdEncoding.EncodeToString(h2.Sum(nil))
}
