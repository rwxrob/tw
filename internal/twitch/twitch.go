package twitch

import (
	"encoding/json"
	"os"
	"os/exec"
)

func channelInfo() map[string]any {
	broadcasterID := os.Getenv("TWITCH_BROADCASTER_ID")
	if broadcasterID == "" {
		return nil
	}
	out, err := exec.Command("twitch", "api", "get", "/channels",
		"-q", "broadcaster_id="+broadcasterID).Output()
	if err != nil {
		return nil
	}
	var result struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(out, &result); err != nil || len(result.Data) == 0 {
		return nil
	}
	return result.Data[0]
}

func Category() string {
	info := channelInfo()
	if info == nil {
		return ""
	}
	if v, ok := info["game_name"].(string); ok {
		return v
	}
	return ""
}

func Title() string {
	info := channelInfo()
	if info == nil {
		return ""
	}
	if v, ok := info["title"].(string); ok {
		return v
	}
	return ""
}
