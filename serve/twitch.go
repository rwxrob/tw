package serve

import (
	"bufio"
	"encoding/json"
	"fmt"
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

// loadTwitchCreds reads TWITCH_CLIENT_ID and TWITCH_TOKEN from env,
// falling back to the twitch-cli config file.
func loadTwitchCreds() (clientID, token string) {
	clientID = os.Getenv("TWITCH_CLIENT_ID")
	token = os.Getenv("TWITCH_TOKEN")
	if clientID != "" && token != "" {
		return
	}
	home := os.Getenv("HOME")
	envFile := filepath.Join(home, "Library", "Application Support", "twitch-cli", ".twitch-cli.env")
	if _, err := os.Stat(envFile); os.IsNotExist(err) {
		envFile = filepath.Join(home, ".config", "twitch-cli", ".twitch-cli.env")
	}
	f, err := os.Open(envFile)
	if err != nil {
		return
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch k {
		case "CLIENTID":
			if clientID == "" {
				clientID = v
			}
		case "ACCESSTOKEN":
			if token == "" {
				token = v
			}
		}
	}
	return
}

func writeTopics(path, current, previous string) error {
	content := current + "\n" + previous + "\n"
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
