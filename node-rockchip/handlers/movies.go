package handlers

import (
  "encoding/json"
  "fmt"
  "log"
  "net/http"
  "os/exec"
  "path"
  "rockchip-node/config"
  "sort"
  "strings"
  "sync"
  "time"
)

// ================================================================
// Simple in-memory cache with RWMutex + background cleanup
// ================================================================

const cacheTTL = 15 * time.Minute

type cacheEntry struct {
  data      []byte
  expiresAt time.Time
}

type memCache struct {
  mu      sync.RWMutex
  entries map[string]cacheEntry
}

func newMemCache() *memCache {
  c := &memCache{entries: make(map[string]cacheEntry)}
  // Background cleanup every 5 minutes — evicts expired entries
  go func() {
    ticker := time.NewTicker(5 * time.Minute)
    for range ticker.C {
      now := time.Now()
      c.mu.Lock()
      for k, e := range c.entries {
        if now.After(e.expiresAt) {
          delete(c.entries, k)
        }
      }
      c.mu.Unlock()
    }
  }()
  return c
}

func (c *memCache) get(key string) ([]byte, bool) {
  c.mu.RLock()
  e, ok := c.entries[key]
  c.mu.RUnlock()
  if !ok || time.Now().After(e.expiresAt) {
    return nil, false
  }
  return e.data, true
}

func (c *memCache) set(key string, data []byte) {
  c.mu.Lock()
  c.entries[key] = cacheEntry{data: data, expiresAt: time.Now().Add(cacheTTL)}
  c.mu.Unlock()
}

// Package-level cache instance (shared across all handlers)
var cache = newMemCache()

// Helper: write cached JSON or compute, cache and write.
func writeJSON(w http.ResponseWriter, data []byte) {
  w.Header().Set("Content-Type", "application/json")
  w.Write(data)
}

// ================================================================
// Domain types
// ================================================================

type MovieItem struct {
  ID       string `json:"id"`
  Name     string `json:"name"`
  Type     string `json:"type"`
  Overview string `json:"overview"`
  Path     string `json:"path"`
  HasImage bool   `json:"has_image"`
  ImageTag string `json:"image_tag"`
}

type EpisodeItem struct {
  ID           string `json:"id"`
  Name         string `json:"name"`
  SeasonNumber int    `json:"season_number"`
  EpisodeIndex int    `json:"episode_index"`
  Path         string `json:"path"`
}

// ================================================================
// Handlers
// ================================================================

func MoviesHandler(w http.ResponseWriter, r *http.Request) {
  const key = "movies_list"

  if cached, ok := cache.get(key); ok {
    writeJSON(w, cached)
    return
  }

  url := fmt.Sprintf(
    "%s/Items?api_key=%s&Recursive=true&IncludeItemTypes=Movie,Series&Fields=Path,Overview,ImageTags&SortBy=SortName&SortOrder=Ascending",
    config.Config.JellyfinURL, config.Config.JellyfinKey,
  )

  log.Printf("[LIBRARY] Fetching movie list from Jellyfin...")
  start := time.Now()
  resp, err := http.Get(url)
  elapsed := time.Since(start)
  if err != nil {
    log.Printf("[LIBRARY] ❌ Error fetching from Jellyfin after %v: %v", elapsed, err)
    http.Error(w, fmt.Sprintf("Error fetching Jellyfin items: %v", err), http.StatusInternalServerError)
    return
  }
  log.Printf("[LIBRARY] ✅ Fetch completed in %v", elapsed)
  defer resp.Body.Close()

  var data struct {
    Items []struct {
      Id        string            `json:"Id"`
      Name      string            `json:"Name"`
      Type      string            `json:"Type"`
      Overview  string            `json:"Overview"`
      Path      string            `json:"Path"`
      ImageTags map[string]string `json:"ImageTags"`
    } `json:"Items"`
  }

  if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
    http.Error(w, fmt.Sprintf("Error decoding Jellyfin data: %v", err), http.StatusInternalServerError)
    return
  }

  // Deduplicate by filename (alternate mount points) and Name (case-insensitive)
  seenFile := make(map[string]bool)
  seenName := make(map[string]bool)
  var results []MovieItem
  for _, item := range data.Items {
    nameLower := strings.ToLower(item.Name)
    fileName  := strings.ToLower(path.Base(item.Path))

    // Se já vimos esse arquivo ou esse nome, ignoramos a duplicata
    if (fileName != "" && fileName != "." && seenFile[fileName]) || seenName[nameLower] {
      continue
    }

    if fileName != "" && fileName != "." {
      seenFile[fileName] = true
    }
    seenName[nameLower] = true

    tag, hasImage := item.ImageTags["Primary"]
    results = append(results, MovieItem{
      ID:       item.Id,
      Name:     item.Name,
      Type:     item.Type,
      Overview: item.Overview,
      Path:     item.Path,
      HasImage: hasImage,
      ImageTag: tag,
    })
  }

  encoded, err := json.Marshal(results)
  if err != nil {
    http.Error(w, fmt.Sprintf("Error encoding response: %v", err), http.StatusInternalServerError)
    return
  }

  cache.set(key, encoded)
  writeJSON(w, encoded)
}

func EpisodesHandler(w http.ResponseWriter, r *http.Request) {
  seriesId := r.URL.Query().Get("seriesId")
  if seriesId == "" {
    http.Error(w, "seriesId parameter is required", http.StatusBadRequest)
    return
  }

  key := "episodes_" + seriesId

  if cached, ok := cache.get(key); ok {
    writeJSON(w, cached)
    return
  }

  url := fmt.Sprintf(
    "%s/Shows/%s/Episodes?api_key=%s&Fields=Path&SortBy=SortName&SortOrder=Ascending",
    config.Config.JellyfinURL, seriesId, config.Config.JellyfinKey,
  )

  log.Printf("[EPISODES] Fetching episodes for series: %s", seriesId)
  start := time.Now()
  resp, err := http.Get(url)
  elapsed := time.Since(start)
  if err != nil {
    log.Printf("[EPISODES] ❌ Error fetching episodes after %v: %v", elapsed, err)
    http.Error(w, fmt.Sprintf("Error fetching episodes: %v", err), http.StatusInternalServerError)
    return
  }
  log.Printf("[EPISODES] ✅ Episodes fetched in %v", elapsed)
  defer resp.Body.Close()

  var data struct {
    Items []struct {
      Id                string `json:"Id"`
      Name              string `json:"Name"`
      ParentIndexNumber int    `json:"ParentIndexNumber"`
      IndexNumber       int    `json:"IndexNumber"`
      Path              string `json:"Path"`
    } `json:"Items"`
  }

  if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
    http.Error(w, fmt.Sprintf("Error decoding episodes: %v", err), http.StatusInternalServerError)
    return
  }

  var episodes []EpisodeItem
  for _, ep := range data.Items {
    if ep.Path == "" {
      continue
    }
    episodes = append(episodes, EpisodeItem{
      ID:           ep.Id,
      Name:         ep.Name,
      SeasonNumber: ep.ParentIndexNumber,
      EpisodeIndex: ep.IndexNumber,
      Path:         ep.Path,
    })
  }

  sort.Slice(episodes, func(i, j int) bool {
    if episodes[i].SeasonNumber != episodes[j].SeasonNumber {
      return episodes[i].SeasonNumber < episodes[j].SeasonNumber
    }
    return episodes[i].EpisodeIndex < episodes[j].EpisodeIndex
  })

  encoded, err := json.Marshal(episodes)
  if err != nil {
    http.Error(w, fmt.Sprintf("Error encoding response: %v", err), http.StatusInternalServerError)
    return
  }

  cache.set(key, encoded)
  writeJSON(w, encoded)
}

func MediaInfoHandler(w http.ResponseWriter, r *http.Request) {
  path := r.URL.Query().Get("path")
  if path == "" {
    http.Error(w, "Path parameter is required", http.StatusBadRequest)
    return
  }

  key := "mediainfo_" + path

  if cached, ok := cache.get(key); ok {
    log.Printf("[MEDIAINFO] CACHE HIT: %s", path)
    writeJSON(w, cached)
    return
  }

  log.Printf("[MEDIAINFO] START analyzing: %s", path)
  start := time.Now()
  cmd := exec.Command("mediainfo", "--Output=JSON", path)
  out, err := cmd.Output()
  elapsed := time.Since(start)

  if err != nil {
    log.Printf("[MEDIAINFO] ❌ ERROR after %v: %v | Path: %s", elapsed, err, path)
    http.Error(w, fmt.Sprintf("MediaInfo error: %v (Path: %s)", err, path), http.StatusInternalServerError)
    return
  }

  log.Printf("[MEDIAINFO] ✅ FINISHED in %v: %s", elapsed, path)
  cache.set(key, out)
  writeJSON(w, out)
}
