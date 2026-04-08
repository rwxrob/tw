package serve

import (
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strconv"

	"github.com/rwxrob/bonzai"
)

var Cmd = &bonzai.Cmd{
	Name:  "serve",
	Alias: "s|d|daemon",
	Short: "run all tw daemons (HTTP, OBS, Twitch, clips)",
	Do:    run,
}

func run(x *bonzai.Cmd, args ...string) error {
	cfg := loadConfig()
	bs := newBelaboxLiveState()

	go startBelabox(cfg, bs)
	go startHTTP(cfg)
	go startOBS(cfg, bs)
	go startTwitchPoller(cfg)
	go startClipsSyncer(cfg)

	log.Printf("serve: all daemons started on port %s", cfg.port)
	select {} // block forever
}

type config struct {
	topicsFile        string
	port              string
	twitchBroadcaster string
	twitchPoll        int
	obsWSURL          string
	obsWSPasswordFile string
	clipsDir          string
	clipsSyncInterval int
	clipsScene        string
	liveScene         string
	liveSceneFile     string
	belaboxStable          int
	belaboxRemoteKeyFile   string

}

func loadConfig() *config {
	c := &config{}

	c.topicsFile = getenv("TOPICS", getenv("TOPIC", filepath.Join(os.Getenv("HOME"), ".topics")))
	c.port = getenv("PORT", "8080")
	c.twitchBroadcaster = os.Getenv("TWITCH_BROADCASTER_ID")
	c.twitchPoll = envInt("TWITCH_POLL", 60)
	c.obsWSURL = getenv("OBS_WS_URL", "ws://127.0.0.1:4455")
	c.obsWSPasswordFile = getenv("OBS_WS_PASSWORD_FILE", filepath.Join(os.Getenv("HOME"), ".config", "obs-websocket", "password"))

	vidDir := "Videos"
	if runtime.GOOS == "darwin" {
		vidDir = "Movies"
	}
	c.clipsDir = getenv("CLIPS_DIR", filepath.Join(os.Getenv("HOME"), vidDir, "twclips"))
	c.clipsSyncInterval = envInt("CLIPS_SYNC_INTERVAL", 3600)
	c.clipsScene = getenv("OBS_CLIPS_SCENE", "Clips")
	c.liveScene = getenv("OBS_LIVE_SCENE", "IRL-Moblin")
	c.liveSceneFile = getenv("OBS_LIVE_SCENE_FILE", filepath.Join(os.Getenv("HOME"), ".local", "state", "tw-live-scene"))
	c.belaboxStable = envInt("OBS_BELABOX_STABLE", 3)
	c.belaboxRemoteKeyFile = getenv("BELABOX_REMOTE_KEY_FILE", filepath.Join(os.Getenv("HOME"), ".config", "tw", "belabox-remote-key"))
	if c.twitchBroadcaster == "" {
		log.Printf("serve: TWITCH_BROADCASTER_ID not set; Twitch integration disabled")
	}

	return c
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
