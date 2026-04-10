package category

import (
	"fmt"
	"strings"

	"github.com/ktr0731/go-fuzzyfinder"
	"github.com/rwxrob/bonzai"
	"github.com/rwxrob/bonzai/cmds/help"
	"github.com/rwxrob/bonzai/edit"
	"github.com/rwxrob/bonzai/vars"
	"github.com/rwxrob/tw/internal/twitch"
)

var Cmd = &bonzai.Cmd{
	Name:    "category",
	Alias:   "cat|c",
	Short:   "pick or set Twitch stream category",
	MaxArgs: -1,
	Cmds:    []*bonzai.Cmd{help.Cmd.AsHidden(), editCmd},
	Do:      run,
	Long: `
Reads ~/.config/tw/categories.yaml and sets the Twitch stream category.

Pass a keyword to fuzzy-match by category name.
With no args, opens an interactive fuzzy finder pre-filled with the
current category.`,
}

var editCmd = &bonzai.Cmd{
	Name:  "edit",
	Short: "open categories.yaml in $EDITOR (or Editor var)",
	Do: func(x *bonzai.Cmd, args ...string) error {
		if e, _ := vars.Data.Get("Editor"); e != "" {
			orig := edit.EditorPriority
			newp := make([]string, 0, len(orig)+1)
			newp = append(newp, orig[0], orig[1], e)
			newp = append(newp, orig[2:]...)
			edit.EditorPriority = newp
			defer func() { edit.EditorPriority = orig }()
		}
		return edit.Files(twitch.CategoriesFile())
	},
}

func run(x *bonzai.Cmd, args ...string) error {
	cats, err := twitch.LoadCategories()
	if err != nil {
		return fmt.Errorf("category: cannot load categories: %w", err)
	}
	if len(cats) == 0 {
		return fmt.Errorf("category: no categories in %s", twitch.CategoriesFile())
	}

	var selected twitch.Category
	if len(args) > 0 {
		keyword := strings.ToLower(strings.Join(args, " "))
		var matched *twitch.Category
		for i, c := range cats {
			if strings.Contains(strings.ToLower(c.Name), keyword) {
				matched = &cats[i]
				break
			}
		}
		if matched == nil {
			return fmt.Errorf("category: no match for %q", keyword)
		}
		selected = *matched
	} else {
		idx, err := fuzzyfinder.Find(cats,
			func(i int) string { return cats[i].Name },
			fuzzyfinder.WithQuery(""),
		)
		if err != nil {
			return nil // user cancelled
		}
		selected = cats[idx]
	}

	broadcasterID, err := twitch.BroadcasterID()
	if err != nil || broadcasterID == "" {
		return fmt.Errorf("category: cannot determine broadcaster ID: %w", err)
	}

	if err := twitch.PatchChannels(broadcasterID, fmt.Sprintf(`{"game_id":"%d"}`, selected.ID)); err != nil {
		return fmt.Errorf("category: %w", err)
	}
	fmt.Println(selected.Name)
	return nil
}
