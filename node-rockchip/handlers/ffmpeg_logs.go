package handlers

import (
  "encoding/json"
  "fmt"
  "log"
  "net/http"
  "os/exec"
  "strings"
  "time"
)

// ================================================================
// FFmpeg Transcoding Logs — Reads FFmpeg activity from Jellyfin journal
// ================================================================

type FFmpegLogsResponse struct {
  Active    bool   `json:"active"`
  Lines     int    `json:"lines"`
  Content   string `json:"content"`
  Timestamp string `json:"timestamp"`
}

func FFmpegLogsHandler(w http.ResponseWriter, r *http.Request) {
  linesParam := r.URL.Query().Get("lines")
  if linesParam == "" {
    linesParam = "500"
  }

  // Read Jellyfin journal logs and grep for ffmpeg-related lines
  // Using journalctl with --no-pager, then filtering in Go for more control
  cmd := exec.Command("journalctl", "-u", "jellyfin", "-n", linesParam, "--no-pager")
  out, err := cmd.CombinedOutput()
  if err != nil {
    log.Printf("[FFMPEG-LOGS] Error reading journalctl: %v", err)
    http.Error(w, fmt.Sprintf("Failed to read Jellyfin logs: %v", err), http.StatusInternalServerError)
    return
  }

  allLines := strings.Split(string(out), "\n")

  // Filter for FFmpeg-related lines
  keywords := []string{
    "ffmpeg",
    "ffprobe",
    "transcode",
    "transcoding",
    "hls_segment",
    "StreamBuilder",
    "PlayMethod",
    "DirectStream",
    "DirectPlay",
    "codec",
    "hwaccel",
    "rkmpp",
    "vaapi",
    "-i file:",
    "matroska",
    "mpegts",
    ".m3u8",
    ".ts\"",
    "hevc",
    "h264",
    "aac",
    "truehd",
    "eac3",
    "dts",
    "copy -tag",
    "segment_filename",
  }

  var filtered []string
  for _, line := range allLines {
    lower := strings.ToLower(line)
    for _, kw := range keywords {
      if strings.Contains(lower, kw) {
        filtered = append(filtered, line)
        break
      }
    }
  }

  // Determine if transcoding is active:
  // Check if any recent lines mention ffmpeg start or active HLS segments
  active := false
  for i := len(filtered) - 1; i >= 0 && i >= len(filtered)-10; i-- {
    lower := strings.ToLower(filtered[i])
    if strings.Contains(lower, "ffmpeg") || strings.Contains(lower, "transcode") {
      active = true
      break
    }
  }

  content := ""
  if len(filtered) > 0 {
    content = strings.Join(filtered, "\n")
  }

  resp := FFmpegLogsResponse{
    Active:    active,
    Lines:     len(filtered),
    Content:   content,
    Timestamp: time.Now().Format(time.RFC3339),
  }

  w.Header().Set("Content-Type", "application/json")
  json.NewEncoder(w).Encode(resp)
}
