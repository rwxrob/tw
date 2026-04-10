package serve

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

func startTwitchPoller(cfg *config) {
	if cfg.twitchBroadcaster == "" {
		log.Printf("twitch: broadcaster ID unavailable; poller disabled")
		return
	}
	if cfg.twitchClientID == "" || cfg.twitchToken == "" {
		log.Printf("twitch: missing client ID or token; poller disabled")
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
	url := "https://api.twitch.tv/helix/channels?broadcaster_id=" + cfg.twitchBroadcaster
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Client-Id", cfg.twitchClientID)
	req.Header.Set("Authorization", "Bearer "+cfg.twitchToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var result struct {
		Data []struct {
			Title string `json:"title"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
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

func writeTopics(path, current, previous string) error {
	content := current + "\n" + previous + "\n"
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
