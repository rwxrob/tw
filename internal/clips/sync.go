package clips

import (
	"database/sql"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/rwxrob/bonzai/vars"
	"github.com/rwxrob/tw/internal/twitch"
	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS clips (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    twitch_id        TEXT    UNIQUE NOT NULL,
    slug             TEXT    NOT NULL,
    title            TEXT,
    broadcaster_name TEXT,
    created_at       TEXT,
    view_count       INTEGER DEFAULT 0,
    duration         REAL    DEFAULT 0,
    thumbnail_url    TEXT,
    downloaded       INTEGER DEFAULT 0,
    failed           INTEGER DEFAULT 0,
    weight           REAL    DEFAULT 1.0
);`

const migrateAddFailed = `ALTER TABLE clips ADD COLUMN failed INTEGER DEFAULT 0`

// SyncClips fetches all Twitch clips for the authenticated broadcaster,
// inserts new ones into clips.db, downloads missing MP4 files, and
// marks them as downloaded. Clips shorter than ClipsMinDuration seconds
// are skipped.
func SyncClips() error {
	dir := clipsDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("clips: mkdir %s: %w", dir, err)
	}

	dbPath := filepath.Join(dir, "clips.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("clips: open db: %w", err)
	}
	defer db.Close()

	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("clips: init schema: %w", err)
	}
	// migrate: ignore error if column already exists
	_, _ = db.Exec(migrateAddFailed)

	// Reset any thumbnails-prod clips that were incorrectly marked
	// as downloaded (they may be JPEG thumbnails saved as .mp4).
	if err := resetFakeDownloads(db, dir); err != nil {
		log.Printf("clips: reset fake downloads: %v", err)
	}

	broadcasterID, err := twitch.BroadcasterID()
	if err != nil {
		return fmt.Errorf("clips: broadcaster ID: %w", err)
	}

	apiClips, err := twitch.GetClips(broadcasterID)
	if err != nil {
		return fmt.Errorf("clips: fetch API: %w", err)
	}

	minDuration := vars.Fetch[float64]("TW_CLIPS_MIN_DURATION", "ClipsMinDuration", 0)
	inserted := 0
	for _, c := range apiClips {
		if minDuration > 0 && c.Duration < minDuration {
			continue
		}
		res, err := db.Exec(
			`INSERT OR IGNORE INTO clips
			 (twitch_id, slug, title, broadcaster_name, created_at, view_count, duration, thumbnail_url)
			 VALUES (?,?,?,?,?,?,?,?)`,
			c.ID, c.ID, c.Title, c.BroadcasterName, c.CreatedAt, c.ViewCount, c.Duration, c.ThumbnailURL,
		)
		if err != nil {
			log.Printf("clips: insert %s: %v", c.ID, err)
			continue
		}
		if n, _ := res.RowsAffected(); n > 0 {
			inserted++
		}
	}

	rows, err := db.Query(`SELECT id, twitch_id, thumbnail_url FROM clips WHERE downloaded=0 AND (failed IS NULL OR failed=0)`)
	if err != nil {
		return fmt.Errorf("clips: query undownloaded: %w", err)
	}

	type pending struct {
		id           int
		twitchID     string
		thumbnailURL string
	}
	var todo []pending
	for rows.Next() {
		var p pending
		if err := rows.Scan(&p.id, &p.twitchID, &p.thumbnailURL); err == nil {
			todo = append(todo, p)
		}
	}
	rows.Close()

	downloaded := 0
	for _, p := range todo {
		var videoURL string
		if strings.Contains(p.thumbnailURL, "thumbnails-prod") {
			u, err := twitch.ClipGQLVideoURL(p.twitchID)
			if err != nil {
				log.Printf("clips: gql %s: %v", p.twitchID, err)
				continue
			}
			videoURL = u
		} else {
			videoURL = twitch.ClipVideoURL(p.thumbnailURL)
			if videoURL == "" {
				log.Printf("clips: unrecognised thumbnail URL for %s — marking failed", p.twitchID)
				_, _ = db.Exec(`UPDATE clips SET failed=1 WHERE id=?`, p.id)
				continue
			}
		}
		dest := filepath.Join(dir, fmt.Sprintf("%d.mp4", p.id))
		if err := downloadFile(videoURL, dest); err != nil {
			log.Printf("clips: download %s: %v", p.twitchID, err)
			if isPermFail(err) {
				_, _ = db.Exec(`UPDATE clips SET failed=1 WHERE id=?`, p.id)
			}
			continue
		}
		if _, err := db.Exec(`UPDATE clips SET downloaded=1 WHERE id=?`, p.id); err != nil {
			log.Printf("clips: mark downloaded %d: %v", p.id, err)
		}
		downloaded++
	}

	log.Printf("clips: sync done — %d from API, %d new, %d downloaded",
		len(apiClips), inserted, downloaded)
	return nil
}

// resetFakeDownloads finds clips marked downloaded whose local file is
// actually a JPEG (from a previous bad run) and resets them.
func resetFakeDownloads(db *sql.DB, dir string) error {
	rows, err := db.Query(`SELECT id FROM clips WHERE downloaded=1`)
	if err != nil {
		return err
	}
	defer rows.Close()
	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()
	for _, id := range ids {
		path := filepath.Join(dir, fmt.Sprintf("%d.mp4", id))
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		header := make([]byte, 12)
		n, _ := f.Read(header)
		f.Close()
		if n >= 3 && header[0] == 0xFF && header[1] == 0xD8 && header[2] == 0xFF {
			os.Remove(path)
			_, _ = db.Exec(`UPDATE clips SET downloaded=0 WHERE id=?`, id)
			log.Printf("clips: reset fake JPEG for id %d", id)
		}
	}
	return nil
}

// permFailError signals a permanent (non-retryable) download failure.
type permFailError struct{ msg string }

func (e permFailError) Error() string { return e.msg }

func isPermFail(err error) bool {
	_, ok := err.(permFailError)
	return ok
}

func downloadFile(url, dest string) error {
	if _, err := os.Stat(dest); err == nil {
		return nil
	}
	resp, err := http.Get(url) //nolint:gosec
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone {
		return permFailError{fmt.Sprintf("HTTP %d fetching %s", resp.StatusCode, url)}
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d fetching %s", resp.StatusCode, url)
	}
	ct := resp.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "image/") {
		return permFailError{fmt.Sprintf("got image Content-Type %q instead of video from %s", ct, url)}
	}
	tmp := dest + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	f.Close()
	return os.Rename(tmp, dest)
}
