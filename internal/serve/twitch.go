package serve

import (
	"log"
	"os"
	"time"

	"github.com/rwxrob/tw/internal/twitch"
)

func startTwitchPoller(cfg *config) {
	if cfg.twitchBroadcaster == "" {
		log.Printf("twitch: broadcaster ID unavailable; poller disabled")
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
	title, err := twitch.ChannelTitle(cfg.twitchBroadcaster)
	if err != nil {
		return err
	}
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
