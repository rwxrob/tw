package category

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/ktr0731/go-fuzzyfinder"
	"github.com/rwxrob/bonzai"
	"github.com/rwxrob/bonzai/cmds/help"
	"github.com/rwxrob/tw/internal/twitch"
)

var Cmd = &bonzai.Cmd{
	Name:    "category",
	Alias:   "cat|c",
	Short:   "pick or set Twitch stream category",
	MaxArgs: -1,
	Cmds:    []*bonzai.Cmd{help.Cmd.AsHidden()},
	Do:      run,
	Long: `
Reads ~/.config/tw/categories.yaml and sets the Twitch stream category.

Pass a keyword to fuzzy-match by category name.
With no args, opens an interactive fuzzy finder pre-filled with the
current category.`,
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
			fuzzyfinder.WithQuery(twitch.GetCategory()),
		)
		if err != nil {
			return nil // user cancelled
		}
		selected = cats[idx]
	}

	broadcasterID := os.Getenv("TWITCH_BROADCASTER_ID")
	if broadcasterID == "" {
		broadcasterID = twitch.BroadcasterID()
	}
	if broadcasterID == "" {
		return fmt.Errorf("category: cannot determine broadcaster ID")
	}

	out, err := exec.Command("twitch", "api", "patch", "channels",
		"-q", "broadcaster_id="+broadcasterID,
		"-b", fmt.Sprintf(`{"game_id":"%d"}`, selected.ID)).CombinedOutput()
	if err != nil && strings.Contains(string(out), `"error"`) {
		return fmt.Errorf("category: twitch api patch failed: %s", out)
	}
	fmt.Println(selected.Name)
	return nil
}
