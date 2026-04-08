package main

import (
	"github.com/rwxrob/bonzai"
	"github.com/rwxrob/tw/category"
	"github.com/rwxrob/tw/clips"
	"github.com/rwxrob/tw/serve"
	"github.com/rwxrob/tw/topic"
	"github.com/rwxrob/tw/what"
)

func main() {
	Cmd.Run()
}

var Cmd = &bonzai.Cmd{
	Name:  "tw",
	Short: "twitch streaming automation",
	Cmds:  []*bonzai.Cmd{serve.Cmd, topic.Cmd, category.Cmd, clips.Cmd, what.Cmd},
	Def:   what.Cmd,
}
