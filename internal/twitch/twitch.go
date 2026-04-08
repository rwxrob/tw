package twitch

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

func Token() string {
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

func Category() string {
	broadcasterID := os.Getenv("TWITCH_BROADCASTER_ID")
	if broadcasterID == "" {
		return ""
	}
	token := Token()
	if token == "" {
		return ""
	}
	req, err := http.NewRequest("GET",
		"https://api.twitch.tv/helix/channels?broadcaster_id="+broadcasterID, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if id := os.Getenv("TWITCH_CLIENT_ID"); id != "" {
		req.Header.Set("Client-Id", id)
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
