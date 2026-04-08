package category

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/ktr0731/go-fuzzyfinder"
	"github.com/rwxrob/bonzai"
)

var Cmd = &bonzai.Cmd{
	Name:  "category",
	Alias: "cat|c",
	Short: "pick and set Twitch stream category",
	Do:    run,
}

func run(x *bonzai.Cmd, args ...string) error {
	catFile := filepath.Join(os.Getenv("HOME"), ".config", "twitch", "categories")
	data, err := os.ReadFile(catFile)
	if err != nil {
		return fmt.Errorf("category: cannot read %s: %w", catFile, err)
	}

	type entry struct {
		name   string
		gameID string
	}
	var entries []entry
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 3 {
			continue
		}
		name := strings.TrimSpace(parts[1])
		gameID := strings.TrimSpace(parts[2])
		if name != "" && gameID != "" {
			entries = append(entries, entry{name, gameID})
		}
	}
	if len(entries) == 0 {
		return fmt.Errorf("category: no categories in %s", catFile)
	}

	// Deduplicate by name (keep first occurrence)
	seen := map[string]bool{}
	var deduped []entry
	for _, e := range entries {
		if !seen[e.name] {
			seen[e.name] = true
			deduped = append(deduped, e)
		}
	}
	entries = deduped

	idx, err := fuzzyfinder.Find(entries, func(i int) string { return entries[i].name })
	if err != nil {
		return nil // user cancelled
	}

	gameID := entries[idx].gameID

	return patchTwitchCategory(gameID)
}

func patchTwitchCategory(gameID string) error {
	broadcasterID := os.Getenv("TWITCH_BROADCASTER_ID")
	if broadcasterID == "" {
		return fmt.Errorf("category: TWITCH_BROADCASTER_ID not set")
	}
	token := twitchToken()
	if token == "" {
		return fmt.Errorf("category: no twitch token")
	}
	clientID := os.Getenv("TWITCH_CLIENT_ID")

	body, _ := json.Marshal(map[string]string{"game_id": gameID})
	req, err := http.NewRequest("PATCH",
		"https://api.twitch.tv/helix/channels?broadcaster_id="+broadcasterID,
		bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	if clientID != "" {
		req.Header.Set("Client-Id", clientID)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("category: twitch patch failed (%d): %s", resp.StatusCode, b)
	}

	fmt.Printf("category: set to game_id=%s\n", gameID)
	return nil
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
