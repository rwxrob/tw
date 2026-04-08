package what

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/rwxrob/bonzai"
)

var Cmd = &bonzai.Cmd{
	Name:  "what",
	Alias: "w",
	Short: "show current topic and Twitch category",
	Do:    run,
}

func run(x *bonzai.Cmd, args ...string) error {
	topicsFile := getTopicsFile()
	topic := readLine1(topicsFile)

	fmt.Println(topic)
	copyToClipboard(topic)

	category := twitchCategory()
	if category != "" {
		fmt.Printf("category: %s\n", category)
	}

	return nil
}

func getTopicsFile() string {
	if v := os.Getenv("TOPICS"); v != "" {
		return v
	}
	if v := os.Getenv("TOPIC"); v != "" {
		return v
	}
	return filepath.Join(os.Getenv("HOME"), ".topics")
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

func copyToClipboard(text string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbcopy")
	default:
		if _, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.Command("xclip", "-selection", "clipboard")
		} else if _, err := exec.LookPath("xsel"); err == nil {
			cmd = exec.Command("xsel", "--clipboard", "--input")
		}
	}
	if cmd == nil {
		return
	}
	cmd.Stdin = strings.NewReader(text)
	_ = cmd.Run()
}

func twitchCategory() string {
	broadcasterID := os.Getenv("TWITCH_BROADCASTER_ID")
	if broadcasterID == "" {
		return ""
	}
	token := twitchToken()
	if token == "" {
		return ""
	}
	clientID := os.Getenv("TWITCH_CLIENT_ID")

	req, err := http.NewRequest("GET",
		"https://api.twitch.tv/helix/channels?broadcaster_id="+broadcasterID, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if clientID != "" {
		req.Header.Set("Client-Id", clientID)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var result struct {
		Data []struct {
			GameName string `json:"game_name"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil || len(result.Data) == 0 {
		return ""
	}
	return result.Data[0].GameName
}

func twitchToken() string {
	if v := os.Getenv("TWITCH_TOKEN"); v != "" {
		return v
	}
	path := filepath.Join(os.Getenv("HOME"), ".config", "twitch", "token")
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
