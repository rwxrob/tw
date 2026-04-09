package main

import (
	"github.com/rwxrob/bonzai"
	"github.com/rwxrob/bonzai/cmds/help"
	"github.com/rwxrob/bonzai/comp"
	"github.com/rwxrob/tw/cachetoken"
	"github.com/rwxrob/tw/category"
	"github.com/rwxrob/tw/clips"
	"github.com/rwxrob/tw/obs"
	"github.com/rwxrob/tw/serve"
	"github.com/rwxrob/tw/topic"
	"github.com/rwxrob/tw/what"
)

func main() {
	Cmd.Exec()
}

var Cmd = &bonzai.Cmd{
	Name:  "tw",
	Short: "twitch livestream automation",
	Comp:  comp.Cmds,
	Cmds:  []*bonzai.Cmd{help.Cmd, serve.Cmd, topic.Cmd, category.Cmd, clips.Cmd, what.Cmd, cachetoken.Cmd, obs.Cmd},
	Def:   what.Cmd,
}
