package torrent

import (
	"context"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"

	"path/filepath"

	"github.com/anacrolix/torrent"
)

type Engine struct {
	client *torrent.Client
}

func NewEngine() (*Engine, error) {
	cfg := torrent.NewDefaultClientConfig()
	
	// Map storage to /tmp (often tmpfs in Linux, avoiding disk I/O)
	tempDir := filepath.Join(os.TempDir(), "torrent-sniffer-data")
	os.MkdirAll(tempDir, 0777)
	cfg.DataDir = tempDir
	cfg.Seed = false
	cfg.NoUpload = true
	cfg.DisableIPv6 = true
	cfg.EstablishedConnsPerTorrent = 15
	cfg.HalfOpenConnsPerTorrent = 10
	cfg.TotalHalfOpenConns = 20
	cfg.ListenPort = 0
	
	cl, err := torrent.NewClient(cfg)
	if err != nil {
		return nil, err
	}
	
	return &Engine{client: cl}, nil
}

// OpenTorrent waits for metadata and returns the torrent and the largest file
func (e *Engine) OpenTorrent(ctx context.Context, magnetURI string) (*torrent.Torrent, *torrent.File, int64, error) {
	log.Printf("[ENGINE] Adding magnet link to client...")
	t, err := e.client.AddMagnet(magnetURI)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("error adding magnet: %w", err)
	}
	
	log.Printf("[ENGINE] Magnet added. Waiting for torrent metadata (infohash: %s)...", t.InfoHash().HexString())
	select {
	case <-t.GotInfo():
		log.Printf("[ENGINE] Metadata received successfully. Name: %s", t.Name())
	case <-ctx.Done():
		t.Drop()
		log.Printf("[ENGINE] Timeout or cancellation while waiting for metadata.")
		return nil, nil, 0, ctx.Err()
	}
	
	// Find the best target file (Prioritize first episode in season packs)
	var largestFile *torrent.File
	var largestSize int64

	type videoFile struct {
		file *torrent.File
		path string
		size int64
	}
	var videos []videoFile

	for _, f := range t.Files() {
		size := f.Length()
		if size > largestSize {
			largestSize = size
			largestFile = f
		}

		path := f.DisplayPath()
		lowerPath := strings.ToLower(path)

		// Ignore small files (< 50MB) and sample files
		if size < 50*1024*1024 || strings.Contains(lowerPath, "sample") {
			continue
		}

		// Keep only video files
		if strings.HasSuffix(lowerPath, ".mkv") || strings.HasSuffix(lowerPath, ".mp4") || strings.HasSuffix(lowerPath, ".avi") {
			videos = append(videos, videoFile{file: f, path: path, size: size})
		}
	}

	var targetFile *torrent.File
	var maxSize int64

	if len(videos) == 0 {
		// Fallback to largest file if no valid video files found
		targetFile = largestFile
		maxSize = largestSize
	} else if len(videos) == 1 {
		// Single video file
		targetFile = videos[0].file
		maxSize = videos[0].size
	} else {
		// Multiple video files (Season pack or Extras)
		
		// Filter out "extras" or "featurettes" to focus on the main content
		var mainVideos []videoFile
		var extraVideos []videoFile

		reExtras := regexp.MustCompile(`(?i)\b(featurettes|extras|bonus|deleted scenes|behind the scenes|trailers)\b`)

		for _, v := range videos {
			if reExtras.MatchString(v.path) {
				extraVideos = append(extraVideos, v)
			} else {
				mainVideos = append(mainVideos, v)
			}
		}

		// If we filtered out everything, fallback to using the extras
		candidateVideos := mainVideos
		if len(candidateVideos) == 0 {
			candidateVideos = extraVideos
		}

		// Sort alphabetically to ensure natural ordering
		for i := 0; i < len(candidateVideos)-1; i++ {
			for j := i + 1; j < len(candidateVideos); j++ {
				if candidateVideos[i].path > candidateVideos[j].path {
					candidateVideos[i], candidateVideos[j] = candidateVideos[j], candidateVideos[i]
				}
			}
		}

		// Try to find the first episode explicitly via regex
		// Matches: S01E01, S1E1, E01, Episode 01, etc.
		reFirstEp := regexp.MustCompile(`(?i)(s\d+e0*1\b|[^a-z]e0*1\b)`)
		
		foundEp1 := false
		for _, v := range candidateVideos {
			if reFirstEp.MatchString(v.path) {
				targetFile = v.file
				maxSize = v.size
				foundEp1 = true
				break
			}
		}

		// If no explicit E01 found, pick the largest video file
		// This is much safer for movies than picking the first alphabetical file
		if !foundEp1 {
			var currentLargest *torrent.File
			var currentMaxSize int64
			for _, v := range candidateVideos {
				if v.size > currentMaxSize {
					currentMaxSize = v.size
					currentLargest = v.file
				}
			}
			targetFile = currentLargest
			maxSize = currentMaxSize
		}
	}

	if targetFile == nil || maxSize == 0 {
		t.Drop()
		return nil, nil, 0, fmt.Errorf("no files found in torrent")
	}
	
	log.Printf("[ENGINE] Target file selected: %s (%.2f MB)", targetFile.DisplayPath(), float64(maxSize)/1024/1024)
	return t, targetFile, maxSize, nil
}

func (e *Engine) Close() {
	if e.client != nil {
		e.client.Close()
	}
}
