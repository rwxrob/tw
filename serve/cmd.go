package serve

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"

	"github.com/rwxrob/bonzai"
	"github.com/rwxrob/bonzai/cmds/help"
)

var Cmd = &bonzai.Cmd{
	Name:  "serve",
	Alias: "s|d|daemon",
	Short: "start HTTP/WebSocket daemon (backgrounds itself)",
	Cmds:  []*bonzai.Cmd{help.Cmd.AsHidden(), stopCmd, tailCmd},
	Def:   &bonzai.Cmd{Do: run},
	Long: `
Starts all background daemons: HTTP overlay server, OBS WebSocket
listener, Twitch title poller, Belabox stats poller, and clip syncer.

Daemonizes itself on first run. Detects and reports if already running
via ~/.local/state/tw.pid. Logs to ~/Library/Logs/tw.log (macOS) or
~/.local/state/tw.log (Linux).

Subcommands:
  stop  send SIGTERM to the running daemon
  tail  tail -f the log file`,
}

var tailCmd = &bonzai.Cmd{
	Name:  "tail",
	Short: "tail -f the log file",
	Do: func(x *bonzai.Cmd, args ...string) error {
		cfg := loadConfig()
		cmd := exec.Command("tail", "-f", cfg.logFile)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	},
}

var stopCmd = &bonzai.Cmd{
	Name:  "stop",
	Short: "stop the running tw serve daemon",
	Do: func(x *bonzai.Cmd, args ...string) error {
		cfg := loadConfig()
		pid := runningPID(cfg.pidFile)
		if pid == 0 {
			fmt.Println("serve: not running")
			return nil
		}
		proc, _ := os.FindProcess(pid)
		if err := proc.Signal(syscall.SIGTERM); err != nil {
			return fmt.Errorf("serve: stop failed: %w", err)
		}
		os.Remove(cfg.pidFile)
		fmt.Printf("serve: stopped (pid %d)\n", pid)
		return nil
	},
}

func run(x *bonzai.Cmd, args ...string) error {
	cfg := loadConfig()

	if pid := runningPID(cfg.pidFile); pid != 0 {
		fmt.Printf("serve: already running (pid %d)\n", pid)
		return nil
	}

	if os.Getenv("_TW_DAEMON") == "" {
		return spawnDaemon(cfg)
	}

	if f, err := os.OpenFile(cfg.logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
		log.SetOutput(f)
	} else {
		log.Printf("serve: cannot open log file %s: %v (logging to stderr)", cfg.logFile, err)
	}

	writePID(cfg.pidFile)
	defer os.Remove(cfg.pidFile)

	bs := newBelaboxLiveState()
	obss := &obsState{}

	go startBelabox(cfg, bs, obss)
	go startHTTP(cfg)
	go startOBS(cfg, bs, obss)
	go startTwitchPoller(cfg)
	go startClipsSyncer(cfg)

	log.Printf("serve: all daemons started on port %s", cfg.port)
	select {} // block forever
}

func spawnDaemon(cfg *config) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(exe, os.Args[1:]...)
	cmd.Env = append(os.Environ(), "_TW_DAEMON=1")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	fmt.Printf("serve: started (pid %d) logging to %s\n", cmd.Process.Pid, cfg.logFile)
	return nil
}

func runningPID(pidFile string) int {
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		os.Remove(pidFile)
		return 0
	}
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		os.Remove(pidFile)
		return 0
	}
	return pid
}

func writePID(pidFile string) {
	_ = os.MkdirAll(filepath.Dir(pidFile), 0755)
	_ = os.WriteFile(pidFile, []byte(strconv.Itoa(os.Getpid())), 0644)
}

type config struct {
	topicsFile            string
	port                  string
	twitchBroadcaster     string
	twitchPoll            int
	obsWSURL              string
	obsWSPasswordFile     string
	clipsDir              string
	clipsSyncInterval     int
	clipsScene            string
	liveScene             string
	liveSceneFile         string
	belaboxStatsURLFile   string
	belaboxPoll           int
	clipsBitrateThreshold int
	logFile               string
	pidFile               string
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
	c.liveScene = getenv("OBS_LIVE_SCENE", "IRL - Moblin")
	c.liveSceneFile = getenv("OBS_LIVE_SCENE_FILE", filepath.Join(os.Getenv("HOME"), ".local", "state", "tw-live-scene"))
	c.belaboxStatsURLFile = getenv("BELABOX_STATS_URL_FILE", filepath.Join(os.Getenv("HOME"), ".config", "tw", "belabox-stats-url"))
	c.belaboxPoll = envInt("BELABOX_POLL", 2)
	c.clipsBitrateThreshold = envInt("CLIPS_BITRATE_THRESHOLD", 600)

	logDefault := filepath.Join(os.Getenv("HOME"), "Library", "Logs", "tw.log")
	if runtime.GOOS != "darwin" {
		logDefault = filepath.Join(os.Getenv("HOME"), ".local", "state", "tw.log")
	}
	c.logFile = getenv("TW_LOG", logDefault)
	c.pidFile = getenv("TW_PID", filepath.Join(os.Getenv("HOME"), ".local", "state", "tw.pid"))

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
