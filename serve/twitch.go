package serve

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func startTwitchPoller(cfg *config) {
	if cfg.twitchBroadcaster == "" {
		log.Printf("twitch: TWITCH_BROADCASTER_ID not set; poller disabled")
		return
	}
	for {
		if err := twitchPollOnce(cfg); err != nil {
			log.Printf("twitch: poll error: %v", err)
		}
		time.Sleep(time.Duration(cfg.twitchPoll) * time.Second)
	}
}

func twitchPollOnce(cfg *config) error {
	token := twitchToken()
	if token == "" {
		return fmt.Errorf("no twitch token available")
	}
	clientID := os.Getenv("TWITCH_CLIENT_ID")

	url := "https://api.twitch.tv/helix/channels?broadcaster_id=" + cfg.twitchBroadcaster
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if clientID != "" {
		req.Header.Set("Client-Id", clientID)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	var result struct {
		Data []struct {
			Title string `json:"title"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return err
	}
	if len(result.Data) == 0 {
		return nil
	}

	title := result.Data[0].Title
	current := readLine1(cfg.topicsFile)
	if title == current || title == "" {
		return nil
	}

	log.Printf("twitch: title changed to: %s", title)
	return writeTopics(cfg.topicsFile, title, current)
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

func writeTopics(path, current, previous string) error {
	content := current + "\n" + previous + "\n"
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
