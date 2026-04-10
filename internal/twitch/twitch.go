package twitch

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/rwxrob/bonzai/vars"
	"golang.org/x/oauth2"
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
	return vars.Fetch[string]("TWITCH_CATEGORIES_FILE", "CategoriesFile", filepath.Join(os.Getenv("HOME"), ".config", "tw", "categories.yaml"))
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

// loadEnvFile parses the twitch-cli env file and returns a key→value map.
func loadEnvFile() map[string]string {
	home := os.Getenv("HOME")
	envFile := filepath.Join(home, "Library", "Application Support", "twitch-cli", ".twitch-cli.env")
	if _, err := os.Stat(envFile); os.IsNotExist(err) {
		envFile = filepath.Join(home, ".config", "twitch-cli", ".twitch-cli.env")
	}
	f, err := os.Open(envFile)
	if err != nil {
		return nil
	}
	defer f.Close()
	m := map[string]string{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		k, v, ok := strings.Cut(scanner.Text(), "=")
		if ok {
			m[k] = v
		}
	}
	return m
}

func LoadCreds() (clientID, token string) {
	m := loadEnvFile()
	clientID = vars.Fetch[string]("TWITCH_CLIENT_ID", "TwitchClientID", m["CLIENTID"])
	token = vars.Fetch[string]("TWITCH_TOKEN", "TwitchToken", m["ACCESSTOKEN"])
	return
}

var httpClient *http.Client
var cachedClientID string

func client() (*http.Client, string, error) {
	if httpClient != nil && cachedClientID != "" {
		return httpClient, cachedClientID, nil
	}
	m := loadEnvFile()
	clientID := vars.Fetch[string]("TWITCH_CLIENT_ID", "TwitchClientID", m["CLIENTID"])
	clientSecret := m["CLIENTSECRET"]
	accessToken := vars.Fetch[string]("TWITCH_TOKEN", "TwitchToken", m["ACCESSTOKEN"])
	refreshToken := m["REFRESHTOKEN"]
	if clientID == "" || accessToken == "" {
		return nil, "", fmt.Errorf("twitch: no credentials")
	}
	conf := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint: oauth2.Endpoint{
			TokenURL: "https://id.twitch.tv/oauth2/token",
		},
	}
	tok := &oauth2.Token{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}
	cachedClientID = clientID
	httpClient = conf.Client(context.Background(), tok)
	return httpClient, cachedClientID, nil
}

func helixGet(path, query string) ([]byte, error) {
	c, clientID, err := client()
	if err != nil {
		return nil, err
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
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, body)
	}
	return io.ReadAll(resp.Body)
}

func helixPatch(path, query, jsonBody string) error {
	c, clientID, err := client()
	if err != nil {
		return err
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
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
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

func BroadcasterID() (string, error) {
	return broadcasterID()
}

func broadcasterID() (string, error) {
	out, err := helixGet("/users", "")
	if err != nil {
		return "", err
	}
	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &result); err != nil || len(result.Data) == 0 {
		return "", fmt.Errorf("twitch: no user data in /users response")
	}
	return result.Data[0].ID, nil
}

func channelInfo() map[string]any {
	id, err := broadcasterID()
	if err != nil || id == "" {
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

// ChannelTitle fetches the current stream title for the given broadcaster ID.
func ChannelTitle(broadcasterID string) (string, error) {
	out, err := helixGet("/channels", "broadcaster_id="+broadcasterID)
	if err != nil {
		return "", err
	}
	var result struct {
		Data []struct {
			Title string `json:"title"`
		} `json:"data"`
	}
	if err := json.Unmarshal(out, &result); err != nil || len(result.Data) == 0 {
		return "", fmt.Errorf("twitch: no channel data")
	}
	return result.Data[0].Title, nil
}

