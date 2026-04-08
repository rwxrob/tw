package serve

import (
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

type obsState struct {
	mu            sync.Mutex
	currentScene  string
	prevScene     string
	debounceTimer *time.Timer
	stableTimer   *time.Timer
}

func startOBS(cfg *config) {
	for {
		if err := obsConnect(cfg); err != nil {
			log.Printf("obs: disconnected: %v; reconnecting in 2s", err)
		}
		time.Sleep(2 * time.Second)
	}
}

func obsConnect(cfg *config) error {
	password := obsLoadPassword(cfg.obsWSPasswordFile)

	c, _, err := websocket.DefaultDialer.Dial(cfg.obsWSURL, nil)
	if err != nil {
		return err
	}
	defer c.Close()

	// Read Hello (op:0)
	_, raw, err := c.ReadMessage()
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
	id := identifyData{RpcVersion: 1, EventSubscriptions: 324}

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
	if err := c.WriteMessage(websocket.TextMessage, identMsg); err != nil {
		return err
	}

	// Read Identified (op:2)
	_, raw, err = c.ReadMessage()
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

	// Send GetCurrentProgramScene request
	reqMsg, _ := json.Marshal(map[string]any{
		"op": 6,
		"d": map[string]any{
			"requestType": "GetCurrentProgramScene",
			"requestId":   "init-scene",
			"requestData": map[string]any{},
		},
	})
	if err := c.WriteMessage(websocket.TextMessage, reqMsg); err != nil {
		return err
	}

	state := &obsState{}

	// Event loop
	for {
		_, raw, err := c.ReadMessage()
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

func obsHandleResponse(cfg *config, state *obsState, c *websocket.Conn, d json.RawMessage) {
	var resp struct {
		RequestType  string `json:"requestType"`
		ResponseData struct {
			CurrentProgramSceneName string `json:"currentProgramSceneName"`
		} `json:"responseData"`
	}
	if err := json.Unmarshal(d, &resp); err != nil {
		return
	}
	if resp.RequestType != "GetCurrentProgramScene" {
		return
	}

	state.mu.Lock()
	defer state.mu.Unlock()
	state.currentScene = resp.ResponseData.CurrentProgramSceneName
	log.Printf("obs: current scene: %s", state.currentScene)

	if state.currentScene == cfg.clipsScene && state.prevScene == "" {
		if data, err := os.ReadFile(cfg.liveSceneFile); err == nil {
			state.prevScene = strings.TrimSpace(string(data))
		} else {
			state.prevScene = cfg.liveScene
		}
	}
}

func obsHandleEvent(cfg *config, state *obsState, c *websocket.Conn, d json.RawMessage) {
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

	case "MediaInputPlaybackEnded":
		var data struct {
			InputName string `json:"inputName"`
		}
		if err := json.Unmarshal(ev.EventData, &data); err != nil {
			return
		}
		if data.InputName != cfg.belaboxSource {
			return
		}
		log.Printf("obs: belabox down")
		state.mu.Lock()
		// Cancel stable timer - Belabox went down again
		if state.stableTimer != nil {
			state.stableTimer.Stop()
			state.stableTimer = nil
		}
		// Start debounce timer only if not already running and not on clips scene
		if state.debounceTimer == nil && state.currentScene != cfg.clipsScene {
			prevScene := state.currentScene
			state.prevScene = prevScene
			_ = os.MkdirAll(filepath.Dir(cfg.liveSceneFile), 0755)
			_ = os.WriteFile(cfg.liveSceneFile, []byte(prevScene+"\n"), 0644)
			log.Printf("obs: belabox ended; debounce %ds before switching to %s", cfg.belaboxDebounce, cfg.clipsScene)
			state.debounceTimer = time.AfterFunc(time.Duration(cfg.belaboxDebounce)*time.Second, func() {
				state.mu.Lock()
				state.debounceTimer = nil
				state.mu.Unlock()
				obsSendRequest(c, "SetCurrentProgramScene", map[string]any{"sceneName": cfg.clipsScene})
				obsSendRequest(c, "TriggerMediaInputAction", map[string]any{
					"inputName":   cfg.vlcSource,
					"mediaAction": "OBS_WEBSOCKET_MEDIA_INPUT_ACTION_RESTART",
				})
				log.Printf("obs: belabox debounce fired; switched to %s", cfg.clipsScene)
			})
		}
		state.mu.Unlock()

	case "MediaInputPlaybackStarted":
		var data struct {
			InputName string `json:"inputName"`
		}
		if err := json.Unmarshal(ev.EventData, &data); err != nil {
			return
		}
		if data.InputName != cfg.belaboxSource {
			return
		}
		log.Printf("obs: belabox up")
		state.mu.Lock()
		if state.currentScene == cfg.clipsScene && state.prevScene != "" {
			// On clips scene: start stable timer to restore
			if state.stableTimer != nil {
				state.stableTimer.Stop()
			}
			targetScene := state.prevScene
			log.Printf("obs: belabox stable timer %ds; will restore to %s", cfg.belaboxStable, targetScene)
			state.stableTimer = time.AfterFunc(time.Duration(cfg.belaboxStable)*time.Second, func() {
				state.mu.Lock()
				state.stableTimer = nil
				state.prevScene = ""
				state.mu.Unlock()
				obsSendRequest(c, "SetCurrentProgramScene", map[string]any{"sceneName": targetScene})
				log.Printf("obs: belabox stable; restored to %s", targetScene)
			})
		} else {
			// Not on clips scene: cancel debounce timer (Belabox came back before switch)
			if state.debounceTimer != nil {
				state.debounceTimer.Stop()
				state.debounceTimer = nil
				log.Printf("obs: belabox restored before switch; debounce cancelled")
			}
		}
		state.mu.Unlock()
	}
}

func obsSendRequest(c *websocket.Conn, reqType string, reqData map[string]any) {
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
	if err := c.WriteMessage(websocket.TextMessage, msg); err != nil {
		log.Printf("obs: write error for %s: %v", reqType, err)
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
