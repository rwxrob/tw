package twitch

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"

	"gopkg.in/yaml.v3"
)

// Category represents one entry in the categories YAML file.
type Category struct {
	Regex string `yaml:"regex"`
	Name  string `yaml:"name"`
	ID    int    `yaml:"id"`
}

// CategoriesFile returns the path to the categories YAML file.
func CategoriesFile() string {
	if v := os.Getenv("TWITCH_CATEGORIES_FILE"); v != "" {
		return v
	}
	return filepath.Join(os.Getenv("HOME"), ".config", "tw", "categories.yaml")
}

// LoadCategories reads and parses the categories YAML file.
func LoadCategories() ([]Category, error) {
	data, err := os.ReadFile(CategoriesFile())
	if err != nil {
		return nil, err
	}
	var cats []Category
	if err := yaml.Unmarshal(data, &cats); err != nil {
		return nil, err
	}
	return cats, nil
}

// MatchCategory returns the first Category whose Regex matches topic
// (case-insensitive). Returns zero value if nothing matches.
func MatchCategory(topic string, cats []Category) (Category, bool) {
	for _, c := range cats {
		re, err := regexp.Compile("(?i)" + c.Regex)
		if err != nil {
			continue
		}
		if re.MatchString(topic) {
			return c, true
		}
	}
	return Category{}, false
}

func BroadcasterID() string {
	return broadcasterID()
}

func broadcasterID() string {
	if id := os.Getenv("TWITCH_BROADCASTER_ID"); id != "" {
		return id
	}
	out, err := exec.Command("twitch", "api", "get", "/users").Output()
	if err != nil {
		return ""
	}
	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &result); err != nil || len(result.Data) == 0 {
		return ""
	}
	return result.Data[0].ID
}

func channelInfo() map[string]any {
	id := broadcasterID()
	if id == "" {
		return nil
	}
	out, err := exec.Command("twitch", "api", "get", "/channels",
		"-q", "broadcaster_id="+id).Output()
	if err != nil {
		return nil
	}
	var result struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(out, &result); err != nil || len(result.Data) == 0 {
		return nil
	}
	return result.Data[0]
}

func GetCategory() string {
	info := channelInfo()
	if info == nil {
		return ""
	}
	if v, ok := info["game_name"].(string); ok {
		return v
	}
	return ""
}

func Title() string {
	info := channelInfo()
	if info == nil {
		return ""
	}
	if v, ok := info["title"].(string); ok {
		return v
	}
	return ""
}
