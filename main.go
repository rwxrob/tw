package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/rwxrob/bonzai"
	"github.com/rwxrob/bonzai/cmds/help"
	"github.com/rwxrob/bonzai/comp"
	"github.com/rwxrob/bonzai/term"
	"github.com/rwxrob/bonzai/vars"
	"github.com/rwxrob/tw/internal/cachetoken"
	"github.com/rwxrob/tw/internal/category"
	"github.com/rwxrob/tw/internal/clips"
	"github.com/rwxrob/tw/internal/login"
	"github.com/rwxrob/tw/internal/obs"
	"github.com/rwxrob/tw/internal/serve"
	"github.com/rwxrob/tw/internal/topic"
	"github.com/rwxrob/tw/internal/what"
)

func init() {
	vars.Cmd.Init = func(x *bonzai.Cmd, args ...string) error {
		if !term.IsInteractive() {
			return nil
		}
		readOps := map[string]bool{
			"get": true, "g": true,
			"data": true,
			"edit": true, "e": true, "ed": true,
			"grep": true,
		}
		op := ""
		if len(args) > 0 {
			op = args[0]
		}
		if op == "" || readOps[op] {
			fmt.Fprintln(os.Stderr, "warning: vars may contain sensitive credentials (TwitchClientID, TwitchToken)")
			fmt.Fprint(os.Stderr, "continue? (y/N) ")
			var answer string
			fmt.Scanln(&answer)
			if strings.ToLower(strings.TrimSpace(answer)) != "y" {
				os.Exit(0)
			}
		}
		return nil
	}
}

func main() {
	Cmd.Exec()
}

var Cmd = &bonzai.Cmd{
	Name:  "tw",
	Short: "twitch livestream automation",
	Comp:  comp.Cmds,
	Cmds:  []*bonzai.Cmd{help.Cmd.AsHidden(), serve.Cmd, topic.Cmd, category.Cmd, clips.Cmd, what.Cmd, cachetoken.Cmd, obs.Cmd, login.Cmd, vars.Cmd},
	Def:   what.Cmd,
}
