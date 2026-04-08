package serve

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

func startClipsSyncer(cfg *config) {
	for {
		syncClips(cfg)
		time.Sleep(time.Duration(cfg.clipsSyncInterval) * time.Second)
	}
}

func syncClips(cfg *config) {
	// Try sync-clips in PATH first, then same dir as binary
	path, err := exec.LookPath("sync-clips")
	if err != nil {
		// Try next to the binary
		exe, err2 := os.Executable()
		if err2 == nil {
			candidate := filepath.Join(filepath.Dir(exe), "sync-clips")
			if _, err3 := os.Stat(candidate); err3 == nil {
				path = candidate
			}
		}
	}
	if path == "" {
		return
	}

	cmd := exec.Command(path)
	cmd.Env = os.Environ()
	if out, err := cmd.CombinedOutput(); err != nil {
		log.Printf("clips: sync-clips error: %v: %s", err, out)
	} else {
		log.Printf("clips: sync-clips done")
	}
}
