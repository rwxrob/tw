package login

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"time"

	"github.com/rwxrob/bonzai"
	"github.com/rwxrob/bonzai/cmds/help"
	"github.com/rwxrob/bonzai/vars"
	"github.com/rwxrob/tw/internal/twitch"
	"golang.org/x/oauth2"
)

var Cmd = &bonzai.Cmd{
	Name:  "login",
	Short: "authenticate with Twitch via OAuth",
	Long: `
Opens a browser to complete the Twitch OAuth2 Authorization Code flow.
Requires TwitchClientID and TwitchClientSecret to be set in vars:

  tw var set TwitchClientID <id>
  tw var set TwitchClientSecret <secret>

Listens on http://localhost:3000 for the OAuth redirect. After login,
TwitchToken and TwitchRefreshToken are stored in vars. The refresh token
is used by the oauth2 client to silently renew the access token — no
manual re-login needed until the refresh token expires.`,
	Cmds: []*bonzai.Cmd{help.Cmd.AsHidden()},
	Do:   run,
}

func run(x *bonzai.Cmd, args ...string) error {
	clientID, _ := vars.Data.Get("TwitchClientID")
	clientSecret, _ := vars.Data.Get("TwitchClientSecret")
	if clientID == "" {
		return fmt.Errorf("login: TwitchClientID not set — run: tw var set TwitchClientID <id>")
	}
	if clientSecret == "" {
		return fmt.Errorf("login: TwitchClientSecret not set — run: tw var set TwitchClientSecret <secret>")
	}

	conf := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Scopes:       []string{"channel:manage:broadcast"},
		RedirectURL:  "http://localhost:3000",
		Endpoint: oauth2.Endpoint{
			AuthURL:  "https://id.twitch.tv/oauth2/authorize",
			TokenURL: "https://id.twitch.tv/oauth2/token",
		},
	}

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	srv := &http.Server{Addr: ":3000", Handler: mux}
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if e := r.URL.Query().Get("error"); e != "" {
			errCh <- fmt.Errorf("login: Twitch auth error: %s", e)
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			errCh <- fmt.Errorf("login: no code in callback")
			return
		}
		fmt.Fprint(w, "<html><body>Login successful. You may close this tab.</body></html>")
		codeCh <- code
	})
	go srv.ListenAndServe() //nolint
	defer srv.Shutdown(context.Background()) //nolint

	authURL := conf.AuthCodeURL("")
	fmt.Printf("Opening browser for Twitch login...\n")
	_ = exec.Command("/usr/bin/open", authURL).Start()
	fmt.Printf("Waiting for OAuth callback on http://localhost:3000 ...\n")

	var code string
	select {
	case code = <-codeCh:
	case err := <-errCh:
		return err
	case <-time.After(2 * time.Minute):
		return fmt.Errorf("login: timed out waiting for browser callback")
	}

	tok, err := conf.Exchange(context.Background(), code)
	if err != nil {
		return fmt.Errorf("login: token exchange failed: %w", err)
	}

	_ = vars.Data.Set("TwitchToken", tok.AccessToken)
	_ = vars.Data.Set("TwitchRefreshToken", tok.RefreshToken)
	twitch.ResetClient()

	id, err := twitch.BroadcasterID()
	if err != nil {
		return fmt.Errorf("login: could not verify broadcaster ID: %w", err)
	}
	fmt.Printf("login: authenticated as broadcaster ID %s\n", id)
	return nil
}
