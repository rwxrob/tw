package topic

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/ktr0731/go-fuzzyfinder"
	"github.com/rwxrob/bonzai"
	"github.com/rwxrob/tw/internal/twitch"
)

var Cmd = &bonzai.Cmd{
	Name:    "topic",
	Alias:   "t",
	Short:   "get or set the current stream topic",
	MaxArgs: -1,
	Do:      run,
}

func run(x *bonzai.Cmd, args ...string) error {
	topicsFile := getTopicsFile()

	if len(args) == 0 {
		return pickTopic(topicsFile)
	}

	arg := strings.Join(args, " ")

	if arg == "-" {
		return swapTopics(topicsFile)
	}

	// fuzzy match against existing topics; fall back to setting as new topic
	if matched := fuzzyMatchTopic(topicsFile, arg); matched != "" {
		return setTopic(topicsFile, matched)
	}

	return setTopic(topicsFile, arg)
}

func getTopicsFile() string {
	if v := os.Getenv("TOPICS"); v != "" {
		return v
	}
	if v := os.Getenv("TOPIC"); v != "" {
		return v
	}
	return filepath.Join(os.Getenv("HOME"), ".topics")
}

func readLine1(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	lines := strings.SplitN(string(data), "\n", 2)
	if len(lines) == 0 {
		return ""
	}
	return strings.TrimRight(lines[0], "\r")
}

func readLine2(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	lines := strings.SplitN(string(data), "\n", 3)
	if len(lines) < 2 {
		return ""
	}
	return strings.TrimRight(lines[1], "\r")
}

func swapTopics(path string) error {
	line2 := readLine2(path)
	if line2 == "" {
		return fmt.Errorf("topic: no previous topic to swap to")
	}
	return setTopic(path, line2)
}

func setTopic(path, newTopic string) error {
	if err := writeTopics(path, newTopic); err != nil {
		return err
	}

	updateTwitchTitle(newTopic)
	updateTwitchCategoryForTopic(newTopic)
	updateGitHubStatus(newTopic)

	fmt.Println(newTopic)
	if cat := twitch.GetCategory(); cat != "" {
		fmt.Println(cat)
	}
	copyToClipboard(newTopic)
	return nil
}

func copyToClipboard(text string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("pbcopy")
	default:
		if _, err := exec.LookPath("xclip"); err == nil {
			cmd = exec.Command("xclip", "-selection", "clipboard")
		} else if _, err := exec.LookPath("xsel"); err == nil {
			cmd = exec.Command("xsel", "--clipboard", "--input")
		}
	}
	if cmd == nil {
		return
	}
	cmd.Stdin = strings.NewReader(text)
	_ = cmd.Run()
}

func writeTopics(path, newTopic string) error {
	// Read existing lines, dedupe, prepend new topic
	var existing []string
	if data, err := os.ReadFile(path); err == nil {
		scanner := bufio.NewScanner(strings.NewReader(string(data)))
		for scanner.Scan() {
			line := strings.TrimRight(scanner.Text(), "\r")
			if line != "" && line != newTopic {
				existing = append(existing, line)
			}
		}
	}
	lines := append([]string{newTopic}, existing...)
	content := strings.Join(lines, "\n") + "\n"
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func updateTwitchTitle(title string) {
	broadcasterID := twitch.BroadcasterID()
	if broadcasterID == "" {
		return
	}
	if len(title) > 140 {
		title = title[:140]
	}
	out, err := exec.Command("twitch", "api", "patch", "channels",
		"-q", "broadcaster_id="+broadcasterID,
		"-b", `{"title":"`+title+`"}`).CombinedOutput()
	if err != nil && strings.Contains(string(out), `"error"`) {
		fmt.Fprintf(os.Stderr, "topic: twitch update failed: %s\n", out)
	}
}

func updateTwitchCategoryForTopic(topic string) {
	cats, err := twitch.LoadCategories()
	if err != nil || len(cats) == 0 {
		return
	}
	cat, ok := twitch.MatchCategory(topic, cats)
	if !ok {
		return
	}
	broadcasterID := twitch.BroadcasterID()
	if broadcasterID == "" {
		return
	}
	out, err := exec.Command("twitch", "api", "patch", "channels",
		"-q", "broadcaster_id="+broadcasterID,
		"-b", fmt.Sprintf(`{"game_id":"%d"}`, cat.ID)).CombinedOutput()
	if err != nil && strings.Contains(string(out), `"error"`) {
		fmt.Fprintf(os.Stderr, "topic: category update failed: %s\n", out)
	}
}

func updateGitHubStatus(topic string) {
	mutation := fmt.Sprintf(`mutation {
  changeUserStatus(input: {message: %q, emoji: ":studio_microphone:", limitedAvailability: false}) {
    status { message }
  }
}`, topic)

	cmd := exec.Command("gh", "api", "graphql", "-f", "query="+mutation)
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "topic: github status update error: %v: %s\n", err, out)
	}
}

func pickTopic(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("topic: cannot read %s: %w", path, err)
	}
	var lines []string
	seen := map[string]bool{}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		if line != "" && !seen[line] {
			seen[line] = true
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 {
		return fmt.Errorf("topic: no topics in %s", path)
	}
	idx, err := fuzzyfinder.Find(lines, func(i int) string { return lines[i] })
	if err != nil {
		return nil // cancelled
	}
	return setTopic(path, lines[idx])
}

func fuzzyMatchTopic(path, keyword string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	kw := strings.ToLower(keyword)
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		if line != "" && strings.Contains(strings.ToLower(line), kw) {
			return line
		}
	}
	return ""
}
