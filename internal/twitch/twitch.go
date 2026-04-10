package twitch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/rwxrob/bonzai/vars"
	"golang.org/x/oauth2"
	"gopkg.in/yaml.v3"
)

// TwitchClip holds clip metadata from GET /helix/clips.
type TwitchClip struct {
	ID              string  `json:"id"`
	BroadcasterID   string  `json:"broadcaster_id"`
	BroadcasterName string  `json:"broadcaster_name"`
	Title           string  `json:"title"`
	CreatedAt       string  `json:"created_at"`
	ViewCount       int     `json:"view_count"`
	Duration        float64 `json:"duration"`
	ThumbnailURL    string  `json:"thumbnail_url"`
}

// ClipVideoURL derives the MP4 download URL from a Twitch thumbnail URL
// by stripping the -preview-WxH.jpg suffix and appending .mp4.
// Returns empty string for new-format thumbnails-prod URLs — use
// ClipGQLVideoURL for those.
func ClipVideoURL(thumbnailURL string) string {
	re := regexp.MustCompile(`-preview-\d+x\d+\.jpg$`)
	if !re.MatchString(thumbnailURL) {
		return ""
	}
	return re.ReplaceAllString(thumbnailURL, ".mp4")
}

// ClipGQLVideoURL fetches a signed MP4 download URL for a Twitch clip
// using the public Twitch GQL API. Required for newer clips whose
// thumbnail URLs are on the twitch-clips-thumbnails-prod CDN.
func ClipGQLVideoURL(slug string) (string, error) {
	const gqlURL = "https://gql.twitch.tv/gql"
	const clientID = "kimne78kx3ncx6brgo4mv6wki5h1ko"

	body := fmt.Sprintf(
		`[{"operationName":"VideoAccessToken_Clip","variables":{"slug":%q},"extensions":{"persistedQuery":{"version":1,"sha256Hash":"36b89d2507fce29e5ca551df756d27c1cfe079e2609642b4390aa4c35796eb11"}}}]`,
		slug,
	)
	req, err := http.NewRequest("POST", gqlURL, strings.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Client-ID", clientID)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var result []struct {
		Data struct {
			Clip struct {
				PlaybackAccessToken struct {
					Signature string `json:"signature"`
					Value     string `json:"value"`
				} `json:"playbackAccessToken"`
				VideoQualities []struct {
					SourceURL string `json:"sourceURL"`
				} `json:"videoQualities"`
			} `json:"clip"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &result); err != nil || len(result) == 0 {
		return "", fmt.Errorf("gql: bad response for %s", slug)
	}
	clip := result[0].Data.Clip
	if len(clip.VideoQualities) == 0 {
		return "", fmt.Errorf("gql: no video qualities for %s", slug)
	}
	sig := clip.PlaybackAccessToken.Signature
	tok := url.QueryEscape(clip.PlaybackAccessToken.Value)
	return fmt.Sprintf("%s?sig=%s&token=%s", clip.VideoQualities[0].SourceURL, sig, tok), nil
}

// GetClips fetches all clips for the broadcaster, paging through all results.
func GetClips(broadcasterID string) ([]TwitchClip, error) {
	var all []TwitchClip
	cursor := ""
	for {
		q := "broadcaster_id=" + broadcasterID + "&first=100"
		if cursor != "" {
			q += "&after=" + cursor
		}
		out, err := helixGet("/clips", q)
		if err != nil {
			return nil, err
		}
		var result struct {
			Data       []TwitchClip `json:"data"`
			Pagination struct {
				Cursor string `json:"cursor"`
			} `json:"pagination"`
		}
		if err := json.Unmarshal(out, &result); err != nil {
			return nil, err
		}
		all = append(all, result.Data...)
		if result.Pagination.Cursor == "" || len(result.Data) == 0 {
			break
		}
		cursor = result.Pagination.Cursor
	}
	return all, nil
}

// Category represents one entry in the categories YAML file.
type Category struct {
	Regex string   `yaml:"regex"`
	Name  string   `yaml:"name"`
	ID    int      `yaml:"id"`
	Tags  []string `yaml:"tags,omitempty"`
}

// CategoriesFile returns the path to the categories YAML file.
func CategoriesFile() string {
	return vars.Fetch[string]("TW_CATEGORIES_FILE", "CategoriesFile", filepath.Join(os.Getenv("HOME"), ".config", "tw", "categories.yaml"))
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

func LoadCreds() (clientID, token string) {
	clientID = vars.Fetch[string]("TW_CLIENT_ID", "TwitchClientID", "")
	token = vars.Fetch[string]("TW_TOKEN", "TwitchToken", "")
	return
}

var httpClient *http.Client
var cachedClientID string

// ResetClient clears the cached HTTP client so the next call rebuilds it
// with fresh credentials from vars.
func ResetClient() {
	httpClient = nil
	cachedClientID = ""
}

// persistingTokenSource wraps an oauth2.TokenSource and saves new
// tokens to bonzai vars whenever a refresh occurs.
type persistingTokenSource struct {
	src      oauth2.TokenSource
	lastHash string
}

func (p *persistingTokenSource) Token() (*oauth2.Token, error) {
	tok, err := p.src.Token()
	if err != nil {
		return nil, err
	}
	h := tok.AccessToken + tok.RefreshToken
	if h != p.lastHash {
		p.lastHash = h
		_ = vars.Data.Set("TwitchToken", tok.AccessToken)
		if tok.RefreshToken != "" {
			_ = vars.Data.Set("TwitchRefreshToken", tok.RefreshToken)
		}
	}
	return tok, nil
}

func client() (*http.Client, string, error) {
	if httpClient != nil && cachedClientID != "" {
		return httpClient, cachedClientID, nil
	}
	clientID := vars.Fetch[string]("TW_CLIENT_ID", "TwitchClientID", "")
	accessToken := vars.Fetch[string]("TW_TOKEN", "TwitchToken", "")
	clientSecret, _ := vars.Data.Get("TwitchClientSecret")
	refreshToken, _ := vars.Data.Get("TwitchRefreshToken")
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
		TokenType:    "bearer",
	}
	ts := conf.TokenSource(context.Background(), tok)
	pts := &persistingTokenSource{src: ts, lastHash: accessToken + refreshToken}

	cachedClientID = clientID
	httpClient = oauth2.NewClient(context.Background(), pts)
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

// GetTags returns the stream tags for the authenticated broadcaster.
func GetTags() []string {
	info := channelInfo()
	if info == nil {
		return nil
	}
	raw, ok := info["tags"].([]any)
	if !ok {
		return nil
	}
	tags := make([]string, 0, len(raw))
	for _, t := range raw {
		if s, ok := t.(string); ok {
			tags = append(tags, s)
		}
	}
	return tags
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
