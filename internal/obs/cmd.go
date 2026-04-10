package obs

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/rwxrob/bonzai"
	"github.com/rwxrob/bonzai/cmds/help"
	"github.com/rwxrob/bonzai/comp"
	"github.com/rwxrob/bonzai/vars"
)

var Cmd = &bonzai.Cmd{
	Name:  "obs",
	Short: "obs setup utilities",
	Comp:  comp.Cmds,
	Cmds:  []*bonzai.Cmd{help.Cmd.AsHidden(), addRTIRLCmd},
}

var addRTIRLCmd = &bonzai.Cmd{
	Name:  "rtirl",
	Short: "add RTIRL map browser source to OBS Moblin scene",
	Do:    runAddRTIRL,
}

func runAddRTIRL(x *bonzai.Cmd, args ...string) error {
	wsURL := vars.Fetch[string]("OBS_WS_URL", "OBSWSAddr", "ws://127.0.0.1:4455")
	passwordFile := vars.Fetch[string]("OBS_WS_PASSWORD_FILE", "OBSPasswordFile", filepath.Join(os.Getenv("HOME"), ".config", "obs-websocket", "password"))
	scene := vars.Fetch[string]("OBS_LIVE_SCENE", "OBSLiveScene", "IRL - Moblin")
	keyFile := vars.Fetch[string]("RTIRL_KEY_FILE", "RTIRLKeyFile", filepath.Join(os.Getenv("HOME"), ".config", "tw", "rtirl-key"))

	key, err := os.ReadFile(keyFile)
	if err != nil {
		return fmt.Errorf("obs: cannot read RTIRL key from %s: %w", keyFile, err)
	}
	rtirl := strings.TrimSpace(string(key))
	url := "https://overlays.rtirl.com/?key=" + rtirl

	password := ""
	if data, err := os.ReadFile(passwordFile); err == nil {
		password = strings.TrimRight(string(data), "\r\n")
	}

	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		return fmt.Errorf("obs: connect: %w", err)
	}
	defer ws.Close()

	// Hello (op:0)
	_, raw, err := ws.ReadMessage()
	if err != nil {
		return err
	}
	var hello struct {
		Op int `json:"op"`
		D  struct {
			Authentication *struct {
				Challenge string `json:"challenge"`
				Salt      string `json:"salt"`
			} `json:"authentication"`
		} `json:"d"`
	}
	if err := json.Unmarshal(raw, &hello); err != nil {
		return err
	}

	// Identify (op:1)
	idD := map[string]any{"rpcVersion": 1, "eventSubscriptions": 0}
	if hello.D.Authentication != nil && password != "" {
		idD["authentication"] = obsAuth(password, hello.D.Authentication.Salt, hello.D.Authentication.Challenge)
	}
	msg, _ := json.Marshal(map[string]any{"op": 1, "d": idD})
	if err := ws.WriteMessage(websocket.TextMessage, msg); err != nil {
		return err
	}

	// Identified (op:2)
	_, raw, err = ws.ReadMessage()
	if err != nil {
		return err
	}
	var identified struct{ Op int `json:"op"` }
	json.Unmarshal(raw, &identified) //nolint
	if identified.Op != 2 {
		return fmt.Errorf("obs: expected op:2 got op:%d", identified.Op)
	}

	// CreateInput
	req, _ := json.Marshal(map[string]any{
		"op": 6,
		"d": map[string]any{
			"requestType": "CreateInput",
			"requestId":   "add-rtirl",
			"requestData": map[string]any{
				"sceneName":  scene,
				"inputName":  "RTIRL Map",
				"inputKind":  "browser_source",
				"inputSettings": map[string]any{
					"url":    url,
					"width":  1920,
					"height": 1080,
					"fps":    30,
					"css":    "",
				},
			},
		},
	})
	if err := ws.WriteMessage(websocket.TextMessage, req); err != nil {
		return err
	}

	// Response (op:7)
	_, raw, err = ws.ReadMessage()
	if err != nil {
		return err
	}
	var resp struct {
		D struct {
			RequestStatus struct {
				Result  bool   `json:"result"`
				Comment string `json:"comment"`
			} `json:"requestStatus"`
		} `json:"d"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return err
	}
	if !resp.D.RequestStatus.Result {
		return fmt.Errorf("obs: CreateInput failed: %s", resp.D.RequestStatus.Comment)
	}

	fmt.Printf("obs: added RTIRL Map browser source to %s\n  url: %s\n", scene, url)
	return nil
}

func obsAuth(password, salt, challenge string) string {
	h := sha256.New()
	h.Write([]byte(password + salt))
	secret := base64.StdEncoding.EncodeToString(h.Sum(nil))
	h2 := sha256.New()
	h2.Write([]byte(secret + challenge))
	return base64.StdEncoding.EncodeToString(h2.Sum(nil))
}
