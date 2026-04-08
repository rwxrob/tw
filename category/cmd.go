package category

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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

	var lines []string
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()
		if line != "" {
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 {
		return fmt.Errorf("category: no categories in %s", catFile)
	}

	// Use fzf to pick
	fzfPath, err := exec.LookPath("fzf")
	if err != nil {
		// No fzf: print list and prompt
		for i, l := range lines {
			fmt.Printf("%d: %s\n", i+1, l)
		}
		return fmt.Errorf("category: fzf not found; install fzf to use this command")
	}

	input := strings.Join(lines, "\n")
	cmd := exec.Command(fzfPath)
	cmd.Stdin = strings.NewReader(input)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return nil // user cancelled
	}

	selected := strings.TrimSpace(string(out))
	if selected == "" {
		return nil
	}

	// Parse tab-separated: "Game Name\tgame_id\tkeywords..."
	parts := strings.Split(selected, "\t")
	if len(parts) < 2 {
		return fmt.Errorf("category: invalid format: %s", selected)
	}
	gameID := strings.TrimSpace(parts[1])

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
