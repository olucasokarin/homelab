package handlers

import (
  "encoding/json"
  "fmt"
  "log"
  "net/http"
  "os/exec"
  "rockchip-node/config"
  "strings"
  "sync"
  "time"
)

// ================================================================
// HDR Profile Scanner — Scans all movies and classifies HDR profiles
// ================================================================

type HDRItem struct {
  Name         string `json:"name"`
  Path         string `json:"path"`
  Codec        string `json:"codec"`
  BitDepth     string `json:"bit_depth"`
  HDRProfile   string `json:"hdr_profile"`
  DVProfile    string `json:"dv_profile"`
  HDRFormatRaw string `json:"hdr_format_raw"`
  ColorSpace   string `json:"color_space"`
  Transfer     string `json:"transfer"`
  MaxCLL       string `json:"max_cll"`
  MaxFALL      string `json:"max_fall"`
  Resolution   string `json:"resolution"`
}

// classifyHDR determines the HDR profile label from mediainfo fields.
// Returns (hdrLabel, dvProfileCode) e.g. ("Dolby Vision", "P7")
func classifyHDR(hdrFormat, hdrFormatProfile, hdrCompat, transfer, colorPrimaries, bitDepth string) (string, string) {
  // Combine HDR_Format and HDR_Format_Profile for comprehensive matching
  combined := strings.ToLower(hdrFormat + " " + hdrFormatProfile)

  // Extract DV profile code from HDR_Format_Profile (e.g. "dvhe.07" → "P7")
  dvCode := extractDVProfile(combined)

  // Dolby Vision — parse profile number from HDR_Format string
  if strings.Contains(combined, "dolby vision") || strings.Contains(combined, "dvhe.") || strings.Contains(combined, "dvh1.") {
    compatLower := strings.ToLower(hdrCompat)
    label := "Dolby Vision"
    if dvCode != "" {
      label += " " + dvCode
    }

    if strings.Contains(compatLower, "hdr10+") {
      return label + " + HDR10+", dvCode
    }
    if strings.Contains(compatLower, "hdr10") {
      return label + " + HDR10", dvCode
    }
    return label, dvCode
  }

  // HDR10+
  if strings.Contains(combined, "hdr10+") || strings.Contains(combined, "hdr10 plus") ||
    strings.Contains(combined, "smpte st 2094") {
    return "HDR10+", ""
  }

  // HDR10 — SMPTE ST 2086 or PQ transfer with BT.2020
  if strings.Contains(combined, "smpte st 2086") || strings.Contains(combined, "hdr10") {
    return "HDR10", ""
  }

  // PQ transfer with BT.2020 but no explicit HDR format → HDR10
  transferLower := strings.ToLower(transfer)
  colorLower := strings.ToLower(colorPrimaries)
  if strings.Contains(transferLower, "pq") && strings.Contains(colorLower, "bt.2020") {
    return "HDR10", ""
  }

  // HLG
  if strings.Contains(transferLower, "hlg") || strings.Contains(combined, "hlg") {
    return "HLG", ""
  }

  return "SDR", ""
}

// extractDVProfile extracts DV profile code like "P7", "P5", "P8.1" from a combined HDR string.
func extractDVProfile(combined string) string {
  // Match dvhe.XX or dvh1.XX patterns
  for _, prefix := range []string{"dvhe.", "dvh1."} {
    idx := strings.Index(combined, prefix)
    if idx >= 0 {
      rest := combined[idx+len(prefix):]
      // Extract digits (e.g. "07" from "dvhe.07.06")
      digits := ""
      for _, c := range rest {
        if c >= '0' && c <= '9' {
          digits += string(c)
        } else {
          break
        }
      }
      if digits != "" {
        // Remove leading zeros
        profileNum := strings.TrimLeft(digits, "0")
        if profileNum == "" {
          profileNum = "0"
        }
        // Check for sub-profile (e.g. P8.1)
        if profileNum == "8" && (strings.Contains(combined, "cross") || strings.Contains(combined, "8.1")) {
          return "P8.1"
        }
        return "P" + profileNum
      }
    }
  }
  return ""
}

func HDRScanHandler(w http.ResponseWriter, r *http.Request) {
  const cacheKey = "hdr_scan_results"

  if cached, ok := cache.get(cacheKey); ok {
    log.Println("[HDR-SCAN] Cache hit, returning cached results")
    writeJSON(w, cached)
    return
  }

  // Step 1: Fetch movie list from Jellyfin (movies only, not series)
  url := fmt.Sprintf(
    "%s/Items?api_key=%s&Recursive=true&IncludeItemTypes=Movie&Fields=Path&SortBy=SortName&SortOrder=Ascending",
    config.Config.JellyfinURL, config.Config.JellyfinKey,
  )

  log.Println("[HDR-SCAN] Fetching movie list from Jellyfin...")
  start := time.Now()
  resp, err := http.Get(url)
  if err != nil {
    log.Printf("[HDR-SCAN] ❌ Error fetching Jellyfin: %v", err)
    http.Error(w, fmt.Sprintf("Error fetching Jellyfin items: %v", err), http.StatusInternalServerError)
    return
  }
  defer resp.Body.Close()

  var data struct {
    Items []struct {
      Name string `json:"Name"`
      Path string `json:"Path"`
    } `json:"Items"`
  }

  if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
    http.Error(w, fmt.Sprintf("Error decoding Jellyfin data: %v", err), http.StatusInternalServerError)
    return
  }

  // Deduplicate by path
  seen := make(map[string]bool)
  type movieEntry struct {
    Name string
    Path string
  }
  var movies []movieEntry
  for _, item := range data.Items {
    if item.Path == "" || seen[item.Path] {
      continue
    }
    seen[item.Path] = true
    movies = append(movies, movieEntry{Name: item.Name, Path: item.Path})
  }

  total := len(movies)
  log.Printf("[HDR-SCAN] Found %d unique movies to scan", total)

  // Step 2: Run mediainfo concurrently (4 workers)
  type scanResult struct {
    Index int
    Item  HDRItem
    Err   error
  }

  results := make([]HDRItem, total)
  var wg sync.WaitGroup
  sem := make(chan struct{}, 4) // concurrency limiter
  errors := 0
  var errMu sync.Mutex

  for i, movie := range movies {
    wg.Add(1)
    go func(idx int, name, filePath string) {
      defer wg.Done()
      sem <- struct{}{}        // acquire slot
      defer func() { <-sem }() // release slot

      cmd := exec.Command("mediainfo", "--Output=JSON", filePath)
      out, err := cmd.Output()
      if err != nil {
        log.Printf("[HDR-SCAN] ⚠️ mediainfo error for %s: %v", name, err)
        errMu.Lock()
        errors++
        errMu.Unlock()
        results[idx] = HDRItem{
          Name:       name,
          Path:       filePath,
          HDRProfile: "Erro",
        }
        return
      }

      // Parse mediainfo JSON
      var mi struct {
        Media struct {
          Track []map[string]interface{} `json:"track"`
        } `json:"media"`
      }
      if err := json.Unmarshal(out, &mi); err != nil {
        log.Printf("[HDR-SCAN] ⚠️ JSON parse error for %s: %v", name, err)
        errMu.Lock()
        errors++
        errMu.Unlock()
        results[idx] = HDRItem{
          Name:       name,
          Path:       filePath,
          HDRProfile: "Erro",
        }
        return
      }

      // Find video track
      item := HDRItem{
        Name: name,
        Path: filePath,
      }

      for _, track := range mi.Media.Track {
        trackType, _ := track["@type"].(string)
        if trackType != "Video" {
          continue
        }

        item.Codec = str(track, "Format")
        if profile := str(track, "Format_Profile"); profile != "" {
          item.Codec += " " + profile
        }
        item.BitDepth = str(track, "BitDepth")
        item.ColorSpace = str(track, "colour_primaries")
        if item.ColorSpace == "" {
          item.ColorSpace = str(track, "ColorPrimaries")
        }
        item.Transfer = str(track, "transfer_characteristics")
        if item.Transfer == "" {
          item.Transfer = str(track, "TransferCharacteristics")
        }
        item.MaxCLL = str(track, "MaxCLL")
        item.MaxFALL = str(track, "MaxFALL")

        width := str(track, "Width")
        height := str(track, "Height")
        if width != "" && height != "" {
          item.Resolution = width + "×" + height
        }

        hdrFormat := str(track, "HDR_Format")
        hdrFormatProfile := str(track, "HDR_Format_Profile")
        hdrCompat := str(track, "HDR_Format_Compatibility")
        item.HDRFormatRaw = hdrFormat
        if hdrFormatProfile != "" {
          item.HDRFormatRaw += " | Profile: " + hdrFormatProfile
        }

        item.HDRProfile, item.DVProfile = classifyHDR(hdrFormat, hdrFormatProfile, hdrCompat, item.Transfer, item.ColorSpace, item.BitDepth)
        break // only first video track
      }

      if item.HDRProfile == "" {
        item.HDRProfile = "SDR"
      }

      results[idx] = item
    }(i, movie.Name, movie.Path)
  }

  wg.Wait()
  elapsed := time.Since(start)
  log.Printf("[HDR-SCAN] ✅ Scan complete: %d movies in %v (%d errors)", total, elapsed, errors)

  encoded, err := json.Marshal(results)
  if err != nil {
    http.Error(w, fmt.Sprintf("Error encoding results: %v", err), http.StatusInternalServerError)
    return
  }

  cache.set(cacheKey, encoded)
  writeJSON(w, encoded)
}

// str safely extracts a string value from a map.
func str(m map[string]interface{}, key string) string {
  if v, ok := m[key]; ok {
    if s, ok := v.(string); ok {
      return s
    }
    // Handle numeric values (BitDepth can come as number)
    return fmt.Sprintf("%v", v)
  }
  return ""
}
