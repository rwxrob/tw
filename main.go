package main

import (
	"github.com/rwxrob/bonzai"
	"github.com/rwxrob/bonzai/cmds/help"
	"github.com/rwxrob/bonzai/comp"
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
