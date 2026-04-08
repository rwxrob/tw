package serve

import (
	"encoding/json"
	"log"
	"os"
	"os/exec"
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
	out, err := exec.Command("twitch", "api", "get", "/channels",
		"-q", "broadcaster_id="+cfg.twitchBroadcaster).Output()
	if err != nil {
		return err
	}

	var result struct {
		Data []struct {
			Title string `json:"title"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
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
