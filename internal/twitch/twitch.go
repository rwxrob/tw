package twitch

import (
	"encoding/json"
	"os"
	"os/exec"
)

func broadcasterID() string {
	if id := os.Getenv("TWITCH_BROADCASTER_ID"); id != "" {
		return id
	}
	out, err := exec.Command("twitch", "api", "get", "/users").Output()
	if err != nil {
		return ""
	}
	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &result); err != nil || len(result.Data) == 0 {
		return ""
	}
	return result.Data[0].ID
}

func channelInfo() map[string]any {
	id := broadcasterID()
	if id == "" {
		return nil
	}
	out, err := exec.Command("twitch", "api", "get", "/channels",
		"-q", "broadcaster_id="+id).Output()
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
