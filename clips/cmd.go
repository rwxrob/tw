package clips

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/rwxrob/bonzai"
	"github.com/rwxrob/bonzai/comp"
	_ "modernc.org/sqlite"
)

var Cmd = &bonzai.Cmd{
	Name:  "clips",
	Alias: "c",
	Short: "manage twitch clips",
	Comp:  comp.Cmds,
	Cmds:  []*bonzai.Cmd{listCmd, syncCmd},
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

func clipsDir() string {
	if v := os.Getenv("CLIPS_DIR"); v != "" {
		return v
	}
	vidDir := "Videos"
	if runtime.GOOS == "darwin" {
		vidDir = "Movies"
	}
	return filepath.Join(os.Getenv("HOME"), vidDir, "twclips")
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
		fmt.Printf("%d\t%s\t%s\n", id, slug, title)
		found = true
	}
	if !found {
		fmt.Println("no clips found")
	}
	return nil
}

func runSync(x *bonzai.Cmd, args ...string) error {
	fmt.Println("clips: sync not yet implemented; run sync-clips manually")
	return nil
}
