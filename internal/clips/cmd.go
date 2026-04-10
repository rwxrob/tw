package clips

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/rwxrob/bonzai"
	"github.com/rwxrob/bonzai/cmds/help"
	"github.com/rwxrob/bonzai/comp"
	"github.com/rwxrob/bonzai/term"
	"github.com/rwxrob/bonzai/to"
	"github.com/rwxrob/bonzai/vars"
	_ "modernc.org/sqlite"
)

var Cmd = &bonzai.Cmd{
	Name:  "clips",
	Alias: "c",
	Short: "manage twitch clips",
	Comp:  comp.Cmds,
	Cmds:  []*bonzai.Cmd{help.Cmd.AsHidden(), listCmd, syncCmd, setCmd},
	Def:   listCmd,
}

var listCmd = &bonzai.Cmd{
	Name:  "list",
	Alias: "ls|l",
	Short: "list downloaded clips",
	Do:    runList,
}

var syncCmd = &bonzai.Cmd{
	Name:  "sync",
	Alias: "s",
	Short: "sync clips from Twitch",
	Do:    runSync,
}

var setCmd = &bonzai.Cmd{
	Name:  "set",
	Short: "set clips configuration values",
	Comp:  comp.Cmds,
	Cmds:  []*bonzai.Cmd{help.Cmd.AsHidden(), setDirCmd},
}

var setDirCmd = &bonzai.Cmd{
	Name:    "dir",
	Short:   "set clips directory path",
	NumArgs: 1,
	Do: func(x *bonzai.Cmd, args ...string) error {
		return vars.Data.Set("ClipsDir", args[0])
	},
}

func clipsDir() string {
	vidDir := "Videos"
	if runtime.GOOS == "darwin" {
		vidDir = "Movies"
	}
	fallback := filepath.Join(os.Getenv("HOME"), vidDir, "twclips")
	return vars.Fetch[string]("TW_CLIPS_DIR", "ClipsDir", fallback)
}

func runList(x *bonzai.Cmd, args ...string) error {
	dbPath := filepath.Join(clipsDir(), "clips.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("clips: cannot open db: %w", err)
	}
	defer db.Close()

	rows, err := db.Query(`SELECT id, slug, title FROM clips WHERE downloaded=1 ORDER BY id DESC LIMIT 50`)
	if err != nil {
		return fmt.Errorf("clips: query error: %w", err)
	}
	defer rows.Close()

	found := false
	for rows.Next() {
		var id int
		var slug, title string
		if err := rows.Scan(&id, &slug, &title); err != nil {
			continue
		}
		if term.IsInteractive() {
			width := int(term.WinSize.Col)
			if width <= 0 {
				width = 80
			}
			fmt.Printf("%s-%s %sid%s: %s%d%s\n",
				term.Dim, term.Reset, term.Cyan, term.Reset, term.Dim, id, term.Reset)
			fmt.Printf("  %sdescription%s:\n%s\n",
				term.Cyan, term.Reset, to.IndentWrapped(4, width, title))
		} else {
			fmt.Printf("%d\t%s\t%s\n", id, slug, title)
		}
		found = true
	}
	if !found {
		fmt.Println("no clips found")
	}
	return nil
}

func runSync(x *bonzai.Cmd, args ...string) error {
	return SyncClips()
}
