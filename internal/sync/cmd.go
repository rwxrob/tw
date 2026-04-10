package sync

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rwxrob/bonzai"
	"github.com/rwxrob/bonzai/cmds/help"
	"github.com/rwxrob/bonzai/vars"
	"github.com/rwxrob/tw/internal/clips"
	"github.com/rwxrob/tw/internal/twitch"
)

var Cmd = &bonzai.Cmd{
	Name:  "sync",
	Short: "sync twitch title, category, tags, and clips",
	Cmds:  []*bonzai.Cmd{help.Cmd.AsHidden()},
	Do:    run,
}

func run(x *bonzai.Cmd, args ...string) error {
	broadcasterID, err := twitch.BroadcasterID()
	if err != nil {
		return fmt.Errorf("sync: broadcaster ID: %w", err)
	}

	topic := currentTopic()
	if topic == "" {
		return fmt.Errorf("sync: no current topic in %s", topicsFile())
	}

	cats, err := twitch.LoadCategories()
	if err != nil || len(cats) == 0 {
		return fmt.Errorf("sync: load categories: %w", err)
	}
	cat, ok := twitch.MatchCategory(topic, cats)
	if !ok {
		return fmt.Errorf("sync: no category match for topic %q", topic)
	}

	tags := mergeTags(cat.Tags, vars.Fetch[string]("TW_TAGS", "Tags", ""))
	patch := map[string]any{
		"title":   topic,
		"game_id": fmt.Sprintf("%d", cat.ID),
	}
	if len(tags) > 0 {
		patch["tags"] = tags
	}
	body, err := json.Marshal(patch)
	if err != nil {
		return err
	}

	if err := twitch.PatchChannels(broadcasterID, string(body)); err != nil {
		return fmt.Errorf("sync: patch channels: %w", err)
	}
	fmt.Printf("synced: %s | %s\n", topic, cat.Name)

	if err := clips.SyncClips(); err != nil {
		return fmt.Errorf("sync: clips: %w", err)
	}

	return nil
}

func currentTopic() string {
	data, err := os.ReadFile(topicsFile())
	if err != nil {
		return ""
	}
	lines := strings.SplitN(string(data), "\n", 2)
	if len(lines) == 0 {
		return ""
	}
	return strings.TrimRight(lines[0], "\r")
}

func topicsFile() string {
	if v := os.Getenv("TW_TOPICS"); v != "" {
		return v
	}
	if v := os.Getenv("TW_TOPIC"); v != "" {
		return v
	}
	if v, err := vars.Data.Get("TopicsFile"); err == nil && v != "" {
		return v
	}
	return filepath.Join(os.Getenv("HOME"), ".config", "tw", "topics.txt")
}

func mergeTags(catTags []string, globalTags string) []string {
	seen := map[string]bool{}
	var out []string
	for _, t := range catTags {
		t = strings.TrimSpace(t)
		if t != "" && !seen[strings.ToLower(t)] {
			seen[strings.ToLower(t)] = true
			out = append(out, t)
		}
	}
	for _, t := range strings.Split(globalTags, ",") {
		t = strings.TrimSpace(t)
		if t != "" && !seen[strings.ToLower(t)] {
			seen[strings.ToLower(t)] = true
			out = append(out, t)
		}
	}
	if len(out) > 10 {
		out = out[:10]
	}
	return out
}
