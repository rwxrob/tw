package serve

import (
	"log"
	"time"

	"github.com/rwxrob/tw/internal/clips"
)

func startClipsSyncer(cfg *config) {
	for {
		if err := clips.SyncClips(); err != nil {
			log.Printf("clips: sync error: %v", err)
		}
		time.Sleep(time.Duration(cfg.clipsSyncInterval) * time.Second)
	}
}
