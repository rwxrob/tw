package topic

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/rwxrob/bonzai"
	"github.com/rwxrob/tw/internal/twitch"
)

var Cmd = &bonzai.Cmd{
	Name:    "topic",
	Alias:   "t",
	Short:   "get or set the current stream topic",
	MaxArgs: 1,
	Do:      run,
}

func run(x *bonzai.Cmd, args ...string) error {
	topicsFile := getTopicsFile()

	if len(args) == 0 {
		topic := readLine1(topicsFile)
		fmt.Println(topic)
		if cat := twitchCategory(); cat != "" {
			fmt.Printf("category: %s\n", cat)
		}
		return nil
	}

	arg := args[0]

	if arg == "-" {
		return swapTopics(topicsFile)
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
	line1 := readLine1(path)
	line2 := readLine2(path)
	return writeTopics(path, line2, line1)
}

func setTopic(path, newTopic string) error {
	old := readLine1(path)
	if err := writeTopics(path, newTopic, old); err != nil {
		return err
	}

	updateTwitchTitle(newTopic)
	updateGitHubStatus(newTopic)

	fmt.Println(newTopic)
	return nil
}

func writeTopics(path, current, previous string) error {
	content := current + "\n" + previous + "\n"
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func updateTwitchTitle(title string) {
	broadcasterID := os.Getenv("TWITCH_BROADCASTER_ID")
	if broadcasterID == "" {
		return
	}
	if len(title) > 140 {
		title = title[:140]
	}
	out, err := exec.Command("twitch", "api", "patch", "channels",
		"-q", "broadcaster_id="+broadcasterID,
		"-b", `{"title":"`+title+`"}`).CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "topic: twitch update failed: %s\n", out)
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

func twitchCategory() string { return twitch.Category() }
