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
	"github.com/rwxrob/bonzai/vars"
	"github.com/rwxrob/tw/internal/twitch"
)

var Cmd = &bonzai.Cmd{
	Name:  "serve",
	Alias: "s|d|daemon",
	Short: "start HTTP/WebSocket daemon (backgrounds itself)",
	Cmds:  []*bonzai.Cmd{help.Cmd.AsHidden(), stopCmd, tailCmd, restartCmd},
	Def:   &bonzai.Cmd{Do: run},
	Long: `
Starts all background daemons: HTTP overlay server, OBS WebSocket
listener, Twitch title poller, Belabox stats poller, and clip syncer.

Daemonizes itself on first run. Detects and reports if already running
via ServePID in vars. Logs to ~/Library/Logs/tw.log (macOS) or
~/.local/state/tw.log (Linux).

Subcommands:
  stop     send SIGTERM to the running daemon
  tail     tail -f the log file
  restart  stop and restart the daemon`,
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

var restartCmd = &bonzai.Cmd{
	Name:  "restart",
	Alias: "r",
	Short: "stop and restart the daemon",
	Do: func(x *bonzai.Cmd, args ...string) error {
		pid := runningPID()
		if pid != 0 {
			proc, _ := os.FindProcess(pid)
			if err := proc.Signal(syscall.SIGTERM); err != nil {
				return fmt.Errorf("serve: stop failed: %w", err)
			}
			_ = vars.Data.Set("ServePID", "")
			fmt.Printf("serve: stopped (pid %d)\n", pid)
		}
		return run(x)
	},
}

var stopCmd = &bonzai.Cmd{
	Name:  "stop",
	Short: "stop the running tw serve daemon",
	Do: func(x *bonzai.Cmd, args ...string) error {
		pid := runningPID()
		if pid == 0 {
			fmt.Println("serve: not running")
			return nil
		}
		proc, _ := os.FindProcess(pid)
		if err := proc.Signal(syscall.SIGTERM); err != nil {
			return fmt.Errorf("serve: stop failed: %w", err)
		}
		_ = vars.Data.Set("ServePID", "")
		fmt.Printf("serve: stopped (pid %d)\n", pid)
		return nil
	},
}

func run(x *bonzai.Cmd, args ...string) error {
	cfg := loadConfig()

	if pid := runningPID(); pid != 0 {
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

	writePID()
	defer vars.Data.Set("PID", "") //nolint

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

func runningPID() int {
	v, err := vars.Data.Get("ServePID")
	if err != nil || v == "" {
		return 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil {
		return 0
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		_ = vars.Data.Set("ServePID", "")
		return 0
	}
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		_ = vars.Data.Set("ServePID", "")
		return 0
	}
	return pid
}

func writePID() {
	_ = vars.Data.Set("ServePID", strconv.Itoa(os.Getpid()))
}

type config struct {
	topicsFile            string
	port                  string
	twitchBroadcaster     string
	twitchPoll            int
	obsWSURL              string
	obsWSPassword         string
	clipsDir              string
	clipsSyncInterval     int
	clipsScene            string
	clipsOfflineDelay     int
	liveScenes            []string
	belaboxStatsURL       string
	belaboxPoll           int
	clipsBitrateThreshold int
	logFile               string
}

func loadConfig() *config {
	c := &config{}

	if v := os.Getenv("TW_TOPICS"); v != "" {
		c.topicsFile = v
	} else if v := os.Getenv("TW_TOPIC"); v != "" {
		c.topicsFile = v
	} else if v, err := vars.Data.Get("TopicsFile"); err == nil && v != "" {
		c.topicsFile = v
	} else {
		c.topicsFile = filepath.Join(os.Getenv("HOME"), ".config", "tw", "topics.txt")
	}

	c.port = vars.Fetch[string]("TW_PORT", "Port", "8080")
	var bidErr error
	c.twitchBroadcaster, bidErr = twitch.BroadcasterID()
	c.twitchPoll = vars.Fetch[int]("TW_TWITCH_POLL", "TwitchPoll", 60)
	c.obsWSURL = vars.Fetch[string]("TW_OBS_WS_URL", "OBSWSAddr", "ws://127.0.0.1:4455")
	c.obsWSPassword = vars.Fetch[string]("TW_OBS_WS_PASSWORD", "OBSPassword", "")

	vidDir := "Videos"
	if runtime.GOOS == "darwin" {
		vidDir = "Movies"
	}
	c.clipsDir = vars.Fetch[string]("TW_CLIPS_DIR", "ClipsDir", filepath.Join(os.Getenv("HOME"), vidDir, "twclips"))
	c.clipsSyncInterval = vars.Fetch[int]("TW_CLIPS_SYNC_INTERVAL", "ClipsSyncInterval", 3600)
	c.clipsScene = vars.Fetch[string]("TW_OBS_CLIPS_SCENE", "OBSClipsScene", "Clips")
	c.clipsOfflineDelay = vars.Fetch[int]("TW_CLIPS_OFFLINE_DELAY", "ClipsOfflineDelay", 5)
	raw := vars.Fetch[string]("TW_OBS_LIVE_SCENES", "OBSLiveScenes", "IRL, IRL - Moblin, IRL - Belabox")
	for _, s := range strings.Split(raw, ",") {
		if s = strings.TrimSpace(s); s != "" {
			c.liveScenes = append(c.liveScenes, s)
		}
	}
	c.belaboxStatsURL = vars.Fetch[string]("TW_BELABOX_STATS_URL", "BelaboxStatsURL", "")
	c.belaboxPoll = vars.Fetch[int]("TW_BELABOX_POLL", "BelaboxPoll", 2)
	c.clipsBitrateThreshold = vars.Fetch[int]("TW_CLIPS_BITRATE_THRESHOLD", "ClipsBitrateThreshold", 200)

	logDefault := filepath.Join(os.Getenv("HOME"), "Library", "Logs", "tw.log")
	if runtime.GOOS != "darwin" {
		logDefault = filepath.Join(os.Getenv("HOME"), ".local", "state", "tw.log")
	}
	c.logFile = vars.Fetch[string]("TW_LOG", "LogFile", logDefault)

	if c.twitchBroadcaster == "" {
		log.Printf("serve: could not resolve broadcaster ID (%v); Twitch integration disabled", bidErr)
	}

	return c
}
