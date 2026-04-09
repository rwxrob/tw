package category

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/ktr0731/go-fuzzyfinder"
	"github.com/rwxrob/bonzai"
)

var Cmd = &bonzai.Cmd{
	Name:    "category",
	Alias:   "cat|c",
	Short:   "pick and set Twitch stream category",
	MaxArgs: -1,
	Do:      run,
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

	seen := map[string]bool{}
	var deduped []entry
	for _, e := range entries {
		if !seen[e.name] {
			seen[e.name] = true
			deduped = append(deduped, e)
		}
	}
	entries = deduped

	var selected entry
	if len(args) > 0 {
		keyword := strings.ToLower(strings.Join(args, " "))
		var matched *entry
		for i, e := range entries {
			if strings.Contains(strings.ToLower(e.name), keyword) {
				matched = &entries[i]
				break
			}
		}
		if matched == nil {
			return fmt.Errorf("category: no match for %q", keyword)
		}
		selected = *matched
	} else {
		idx, err := fuzzyfinder.Find(entries, func(i int) string { return entries[i].name })
		if err != nil {
			return nil // user cancelled
		}
		selected = entries[idx]
	}

	broadcasterID := os.Getenv("TWITCH_BROADCASTER_ID")
	if broadcasterID == "" {
		return fmt.Errorf("category: TWITCH_BROADCASTER_ID not set")
	}

	out, err := exec.Command("twitch", "api", "patch", "channels",
		"-q", "broadcaster_id="+broadcasterID,
		"-b", `{"game_id":"`+selected.gameID+`"}`).CombinedOutput()
	if err != nil && strings.Contains(string(out), `"error"`) {
		return fmt.Errorf("category: twitch api patch failed: %s", out)
	}
	fmt.Println(selected.name)
	return nil
}
