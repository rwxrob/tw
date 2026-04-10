package serve

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"
)

type belaboxLiveState struct {
	mu     sync.Mutex
	isLive bool
	ch     chan bool
}

func newBelaboxLiveState() *belaboxLiveState {
	return &belaboxLiveState{ch: make(chan bool, 4)}
}

func (bs *belaboxLiveState) set(live bool) {
	bs.mu.Lock()
	prev := bs.isLive
	bs.isLive = live
	bs.mu.Unlock()
	if live != prev {
		log.Printf("belabox: live=%v", live)
		select {
		case bs.ch <- live:
		default:
		}
	}
}

func (bs *belaboxLiveState) get() bool {
	bs.mu.Lock()
	defer bs.mu.Unlock()
	return bs.isLive
}

func startBelabox(cfg *config, bs *belaboxLiveState, obss *obsState) {
	url := cfg.belaboxStatsURL
	if url == "" {
		log.Printf("belabox: no stats URL configured; Belabox integration disabled")
		return
	}
	log.Printf("belabox: polling %s every %ds", url, cfg.belaboxPoll)
	ticker := time.NewTicker(time.Duration(cfg.belaboxPoll) * time.Second)
	defer ticker.Stop()
	var belowSince time.Time
	sustainDur := time.Duration(cfg.clipsOfflineDelay) * time.Second
	for range ticker.C {
		scene := obss.scene()
		if !isLiveScene(scene, cfg.liveScenes) && scene != cfg.clipsScene {
			continue
		}
		live := belaboxFetch(url, cfg.clipsBitrateThreshold)
		if live {
			belowSince = time.Time{}
			bs.set(true)
		} else {
			if belowSince.IsZero() {
				belowSince = time.Now()
			} else if time.Since(belowSince) >= sustainDur {
				bs.set(false)
			}
		}
	}
}

func belaboxFetch(url string, bitrateThreshold int) bool {
	resp, err := http.Get(url) //nolint
	if err != nil {
		log.Printf("belabox: fetch error: %v", err)
		return false
	}
	defer resp.Body.Close()

	var data struct {
		Publishers map[string]struct {
			Connected bool `json:"connected"`
			Bitrate   int  `json:"bitrate"`
		} `json:"publishers"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		log.Printf("belabox: parse error: %v", err)
		return false
	}

	for name, pub := range data.Publishers {
		log.Printf("belabox: %s connected=%v bitrate=%d", name, pub.Connected, pub.Bitrate)
		if pub.Connected && pub.Bitrate >= bitrateThreshold {
			return true
		}
	}
	return false
}
