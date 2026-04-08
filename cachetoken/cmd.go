package cachetoken

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/rwxrob/bonzai"
)

var Cmd = &bonzai.Cmd{
	Name:  "cache-token",
	Alias: "token",
	Short: "refresh Twitch user token with broadcast scope",
	Do:    run,
}

// sensitive substrings — any output line containing these is suppressed
var sensitive = []string{
	"token", "Token", "TOKEN",
	"secret", "Secret", "SECRET",
	"password", "Password",
	"http://", "https://",
}

func isSensitive(line string) bool {
	for _, s := range sensitive {
		if strings.Contains(line, s) {
			return true
		}
	}
	return false
}

func run(x *bonzai.Cmd, args ...string) error {
	cmd := exec.Command("twitch", "token", "-u", "-s", "channel:manage:broadcast")
	cmd.Stdin = os.Stdin
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	scanner := bufio.NewScanner(&buf)
	for scanner.Scan() {
		line := scanner.Text()
		if !isSensitive(line) {
			fmt.Println(line)
		}
	}
	return err
}
