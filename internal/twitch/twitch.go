package twitch

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

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

// LoadCreds returns (clientID, token) from env vars or the twitch-cli
// config file at ~/Library/Application Support/twitch-cli/.twitch-cli.env.
func LoadCreds() (clientID, token string) {
	clientID = os.Getenv("TWITCH_CLIENT_ID")
	token = os.Getenv("TWITCH_TOKEN")
	if clientID != "" && token != "" {
		return
	}
	home := os.Getenv("HOME")
	envFile := filepath.Join(home, "Library", "Application Support", "twitch-cli", ".twitch-cli.env")
	if _, err := os.Stat(envFile); os.IsNotExist(err) {
		envFile = filepath.Join(home, ".config", "twitch-cli", ".twitch-cli.env")
	}
	f, err := os.Open(envFile)
	if err != nil {
		return
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		k, v, ok := strings.Cut(scanner.Text(), "=")
		if !ok {
			continue
		}
		switch k {
		case "CLIENTID":
			if clientID == "" {
				clientID = v
			}
		case "ACCESSTOKEN":
			if token == "" {
				token = v
			}
		}
	}
	return
}

func helixGet(path, query string) ([]byte, error) {
	clientID, token := LoadCreds()
	if clientID == "" || token == "" {
		return nil, fmt.Errorf("twitch: no credentials")
	}
	u := "https://api.twitch.tv/helix" + path
	if query != "" {
		u += "?" + query
	}
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Client-Id", clientID)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func helixPatch(path, query, jsonBody string) error {
	clientID, token := LoadCreds()
	if clientID == "" || token == "" {
		return fmt.Errorf("twitch: no credentials")
	}
	u := "https://api.twitch.tv/helix" + path
	if query != "" {
		u += "?" + query
	}
	req, err := http.NewRequest("PATCH", u, strings.NewReader(jsonBody))
	if err != nil {
		return err
	}
	req.Header.Set("Client-Id", clientID)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 204 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, body)
	}
	return nil
}

// PatchChannels updates fields on the channel via PATCH /channels.
func PatchChannels(broadcasterID, jsonBody string) error {
	return helixPatch("/channels", "broadcaster_id="+broadcasterID, jsonBody)
}

func BroadcasterID() string {
	return broadcasterID()
}

func broadcasterID() string {
	out, err := helixGet("/users", "")
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
	out, err := helixGet("/channels", "broadcaster_id="+id)
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
