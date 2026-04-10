package login

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/rwxrob/bonzai"
	"github.com/rwxrob/bonzai/cmds/help"
	"github.com/rwxrob/tw/internal/twitch"
)

var Cmd = &bonzai.Cmd{
	Name:  "login",
	Short: "authenticate with Twitch via OAuth",
	Long: `
Runs the Twitch OAuth user token flow (twitch token -u) and stores
credentials in the twitch-cli config file.

A user access token (not an app token) is required because:
- GET /helix/users without query params only returns the authenticated
  user's info when called with a user token; app tokens require an
  explicit user ID or login query param and cannot self-identify.
- PATCH /helix/channels requires a user token with the
  channel:manage:broadcast scope.

Running "twitch configure" or "twitch token" (without -u) produces an
app access token which will fail both of the above calls.

After login, the stored refresh token is used by golang.org/x/oauth2 to
auto-refresh the access token silently — no manual re-login needed until
the refresh token itself expires (typically 30+ days of inactivity).

After login succeeds, verifies the connection by resolving and
displaying the authenticated broadcaster ID.`,
	Cmds: []*bonzai.Cmd{help.Cmd.AsHidden()},
	Do:   run,
}

var sensitive = []string{
	"token", "Token", "TOKEN",
	"secret", "Secret", "SECRET",
	"password", "Password",
	"http://", "https://",
}

func run(x *bonzai.Cmd, args ...string) error {
	cmd := exec.Command("twitch", "token", "-u", "-s", "channel:manage:broadcast")
	cmd.Stdin = os.Stdin
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		fmt.Print(buf.String())
		return err
	}
	scanner := bufio.NewScanner(&buf)
	for scanner.Scan() {
		line := scanner.Text()
		skip := false
		for _, s := range sensitive {
			if strings.Contains(line, s) {
				skip = true
				break
			}
		}
		if !skip {
			fmt.Println(line)
		}
	}
	id, err := twitch.BroadcasterID()
	if err != nil {
		return fmt.Errorf("login: could not verify broadcaster ID: %w", err)
	}
	fmt.Printf("login: authenticated as broadcaster ID %s\n", id)
	return nil
}
