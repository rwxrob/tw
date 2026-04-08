package what

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/rwxrob/bonzai"
	"github.com/rwxrob/tw/internal/twitch"
)

var Cmd = &bonzai.Cmd{
	Name:  "what",
	Alias: "w",
	Short: "show current topic and Twitch category",
	Do:    run,
}

func run(x *bonzai.Cmd, args ...string) error {
	topicsFile := getTopicsFile()
	topic := readLine1(topicsFile)

	fmt.Println(topic)
	copyToClipboard(topic)

	if cat := twitch.Category(); cat != "" {
		fmt.Println(cat)
	}

	return nil
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
