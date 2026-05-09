package handlers

import (
  "encoding/json"
  "fmt"
  "net/http"
  "os/exec"
  "regexp"
  "strings"
  "time"
)

// ================================================================
// Jellyfin Log Analyzer — Diagnostica problemas de playback
// ================================================================

type AnalyzeIssue struct {
  Severity     string   `json:"severity"`
  Category     string   `json:"category"`
  Title        string   `json:"title"`
  Description  string   `json:"description"`
  MediaName    string   `json:"media_name,omitempty"`
  ClientInfo   string   `json:"client_info,omitempty"`
  ClientType   string   `json:"client_type,omitempty"`
  RelevantLogs []string `json:"relevant_logs"`
  Suggestion   string   `json:"suggestion"`
}

type AnalyzeSession struct {
  MediaName  string `json:"media_name"`
  Client     string `json:"client"`
  Device     string `json:"device"`
  ClientType string `json:"client_type,omitempty"`
  PlayMethod string `json:"play_method"`
  Timestamp  string `json:"timestamp,omitempty"`
}

type AnalyzeResponse struct {
  Timestamp  string           `json:"timestamp"`
  TotalLines int              `json:"total_lines"`
  Issues     []AnalyzeIssue   `json:"issues"`
  Sessions   []AnalyzeSession `json:"sessions"`
  RawSnippet string           `json:"raw_snippet"`
}

// Compiled regex patterns for log analysis
var (
  // FFmpeg crash / exit
  reFFmpegExit = regexp.MustCompile(`(?i)ffmpeg.*exited?\s+with\s+code\s+(\d+)`)
  reFFmpegErr  = regexp.MustCompile(`(?i)(ffmpeg|ffprobe).*(?:error|fail|crash|killed|abort)`)

  // Transcoding decisions
  reTranscode    = regexp.MustCompile(`(?i)StreamBuilder.*(?:transcode|transcoding)`)
  reDirectStream = regexp.MustCompile(`(?i)StreamBuilder.*(?:direct\s*(?:stream|play))`)
  rePlayMethod   = regexp.MustCompile(`(?i)PlayMethod[:\s]+(DirectStream|DirectPlay|Transcode)`)

  // Codec detection
  reCodecUnsupported = regexp.MustCompile(`(?i)(codec|format|profile).*(?:not\s+supported|unsupported|incompatible)`)
  reCodecInfo        = regexp.MustCompile(`(?i)(hevc|h\.?265|h\.?264|avc|av1|vp9|mpeg[24]|dts|truehd|eac3|aac|flac|opus)`)
  reHWAccelFail      = regexp.MustCompile(`(?i)(hardware.*(?:accel|decode|encode).*(?:fail|error|not\s+available|unsupported)|vaapi.*error|qsv.*error|nvdec.*error|rkmpp.*error)`)

  // Permission / file errors
  rePermission = regexp.MustCompile(`(?i)(permission\s+denied|access\s+denied|unauthorized)`)
  reFileNotFound = regexp.MustCompile(`(?i)(file\s+(?:does\s+not|doesn'?t)\s+exist|no\s+such\s+file|not\s+found|FileNotFoundException)`)
  reIOError = regexp.MustCompile(`(?i)(i/?o\s+error|read\s+error|broken\s+pipe|connection\s+reset)`)

  // Subtitle issues
  reSubtitleErr = regexp.MustCompile(`(?i)(subtitle|srt|ass|ssa|sub).*(?:error|fail|extract|burn|convert)`)

  // Session / playback events
  rePlaybackStart = regexp.MustCompile(`(?i)playback.*start`)
  rePlaybackStop  = regexp.MustCompile(`(?i)playback.*stop`)

  // Media name extraction — tries to get the item name from log lines
  reMediaItem = regexp.MustCompile(`(?i)(?:playing|item|media|file|path)[:\s]+.*[/\\]([^/\\]+?)(?:\.\w{2,4})?(?:\s|$|")`)
  reItemName  = regexp.MustCompile(`(?i)(?:'|"|item\s*(?:name)?[:\s]+)([^'"]+?)(?:'|"|$)`)

  // Client / device extraction — Jellyfin log format patterns
  reClient     = regexp.MustCompile(`(?i)(?:client|app|user[\s-]?agent)[:\s]+"?([^"\n,]+)`)
  reDevice     = regexp.MustCompile(`(?i)(?:device(?:Name|Id)?)[:\s]+"?([^"\n,]+)`)
  reJfClient   = regexp.MustCompile(`(?i)"(?:Client|AppName)"\s*:\s*"([^"]+)"`)
  reJfDevice   = regexp.MustCompile(`(?i)"(?:DeviceName|Device)"\s*:\s*"([^"]+)"`)
  reJfUser     = regexp.MustCompile(`(?i)"(?:User(?:Name)?|user)"\s*:\s*"([^"]+)"`)
  reUserBy     = regexp.MustCompile(`(?i)by\s+user\s+"?([\w]+)"?`)

  // Timestamp extraction from journal lines
  reTimestamp = regexp.MustCompile(`^(\w{3}\s+\d{1,2}\s+\d{2}:\d{2}:\d{2})`)
)

func JellyfinAnalyzeHandler(w http.ResponseWriter, r *http.Request) {
  linesParam := r.URL.Query().Get("lines")
  if linesParam == "" {
    linesParam = "500"
  }

  // Read Jellyfin journal logs
  cmd := exec.Command("journalctl", "-u", "jellyfin", "-n", linesParam, "--no-pager")
  out, err := cmd.CombinedOutput()
  if err != nil {
    http.Error(w, fmt.Sprintf("Failed to read Jellyfin logs: %v", err), http.StatusInternalServerError)
    return
  }

  logText := string(out)
  lines := strings.Split(logText, "\n")

  response := AnalyzeResponse{
    Timestamp:  time.Now().Format(time.RFC3339),
    TotalLines: len(lines),
    Issues:     []AnalyzeIssue{},
    Sessions:   []AnalyzeSession{},
  }

  // Build raw snippet (last 40 lines)
  snippetStart := 0
  if len(lines) > 40 {
    snippetStart = len(lines) - 40
  }
  response.RawSnippet = strings.Join(lines[snippetStart:], "\n")

  // Track what we've already reported to avoid duplicates
  seenIssues := make(map[string]bool)

  // Helper: enrich issue with client info from wider context
  enrichIssue := func(issue *AnalyzeIssue, lineIdx int) {
    wideCtx := getContext(lines, lineIdx, 10)
    ci := extractClientInfo(wideCtx)
    di := extractDeviceInfo(wideCtx)
    if ci != "" {
      issue.ClientInfo = ci
    }
    if di != "" {
      if issue.ClientInfo != "" {
        issue.ClientInfo += " — " + di
      } else {
        issue.ClientInfo = di
      }
    }
    issue.ClientType = classifyClient(ci, di)
  }

  // Scan every line for patterns
  for i, line := range lines {
    trimmed := strings.TrimSpace(line)
    if trimmed == "" {
      continue
    }

    // ── FFmpeg Exit Code ──
    if matches := reFFmpegExit.FindStringSubmatch(trimmed); matches != nil {
      key := "ffmpeg_exit_" + matches[1]
      if !seenIssues[key] {
        seenIssues[key] = true
        context := getContext(lines, i, 3)
        issue := AnalyzeIssue{
          Severity:     "error",
          Category:     "transcoding",
          Title:        fmt.Sprintf("FFmpeg encerrou com código %s", matches[1]),
          Description:  fmt.Sprintf("O processo FFmpeg do Jellyfin crashou ao tentar transcodificar. Exit code %s indica que a conversão de vídeo falhou durante a reprodução.", matches[1]),
          MediaName:    extractMediaName(context),
          RelevantLogs: context,
          Suggestion:   "Isso geralmente ocorre quando o codec do vídeo não é suportado para transcoding no hardware atual (Rock 3A). Tente desativar transcoding de hardware nas configurações do Jellyfin, ou verifique se o vídeo usa HEVC/H.265 que pode não ter suporte de hardware.",
        }
        enrichIssue(&issue, i)
        response.Issues = append(response.Issues, issue)
      }
    }

    // ── FFmpeg general errors ──
    if reFFmpegErr.MatchString(trimmed) && !reFFmpegExit.MatchString(trimmed) {
      key := "ffmpeg_err_" + fmt.Sprintf("%d", i/10)
      if !seenIssues[key] {
        seenIssues[key] = true
        context := getContext(lines, i, 2)
        issue := AnalyzeIssue{
          Severity:     "error",
          Category:     "transcoding",
          Title:        "Erro no processo FFmpeg",
          Description:  "Um erro genérico do FFmpeg foi detectado nos logs. Isso pode indicar falha de transcoding, problema de codec ou falta de recursos.",
          MediaName:    extractMediaName(context),
          RelevantLogs: context,
          Suggestion:   "Verifique se o arquivo de mídia está acessível e se os codecs usados são compatíveis com a configuração do Jellyfin.",
        }
        enrichIssue(&issue, i)
        response.Issues = append(response.Issues, issue)
      }
    }

    // ── Hardware acceleration failure ──
    if reHWAccelFail.MatchString(trimmed) {
      key := "hwaccel_fail"
      if !seenIssues[key] {
        seenIssues[key] = true
        context := getContext(lines, i, 3)
        issue := AnalyzeIssue{
          Severity:     "error",
          Category:     "hardware",
          Title:        "Falha na aceleração de hardware",
          Description:  "O Jellyfin tentou usar aceleração de hardware (VAAPI/RKMPP/QSV) para transcoding mas falhou. Isso resulta em tela preta ou vídeo que não carrega.",
          MediaName:    extractMediaName(context),
          RelevantLogs: context,
          Suggestion:   "No Rock 3A, verifique se o driver RKMPP está ativo. Tente desativar a aceleração de hardware nas configurações de Playback do Jellyfin (Dashboard → Playback → desmarcar Hardware Acceleration).",
        }
        enrichIssue(&issue, i)
        response.Issues = append(response.Issues, issue)
      }
    }

    // ── Codec not supported ──
    if reCodecUnsupported.MatchString(trimmed) {
      key := "codec_unsupported_" + fmt.Sprintf("%d", i/10)
      if !seenIssues[key] {
        seenIssues[key] = true
        context := getContext(lines, i, 3)
        codecMatches := reCodecInfo.FindAllString(strings.Join(context, " "), -1)
        codecStr := strings.Join(unique(codecMatches), ", ")
        desc := "O Jellyfin detectou que um codec não é suportado pelo dispositivo cliente."
        if codecStr != "" {
          desc = fmt.Sprintf("O Jellyfin detectou que o codec %s não é suportado pelo dispositivo cliente.", strings.ToUpper(codecStr))
        }
        issue := AnalyzeIssue{
          Severity:     "warning",
          Category:     "codec",
          Title:        "Codec não suportado pelo dispositivo",
          Description:  desc,
          MediaName:    extractMediaName(context),
          RelevantLogs: context,
          Suggestion:   "Sua TV pode não suportar este codec nativamente, forçando o Jellyfin a transcodificar. Verifique se a TV TCL 55C6K suporta o codec diretamente nas configurações de playback do app Jellyfin.",
        }
        enrichIssue(&issue, i)
        response.Issues = append(response.Issues, issue)
      }
    }

    // ── Permission denied ──
    if rePermission.MatchString(trimmed) {
      key := "permission"
      if !seenIssues[key] {
        seenIssues[key] = true
        context := getContext(lines, i, 2)
        issue := AnalyzeIssue{
          Severity: "error", Category: "permission",
          Title: "Permissão negada ao acessar arquivo",
          Description: "O Jellyfin não tem permissão para ler o arquivo de mídia. Isso causa tela preta pois o servidor não consegue sequer iniciar o streaming.",
          MediaName: extractMediaName(context), RelevantLogs: context,
          Suggestion: "Verifique as permissões do diretório de mídia. Execute: sudo chown -R jellyfin:jellyfin /caminho/da/mídia ou ajuste as permissões com chmod.",
        }
        enrichIssue(&issue, i)
        response.Issues = append(response.Issues, issue)
      }
    }

    // ── File not found ──
    if reFileNotFound.MatchString(trimmed) {
      key := "file_not_found_" + fmt.Sprintf("%d", i/10)
      if !seenIssues[key] {
        seenIssues[key] = true
        context := getContext(lines, i, 2)
        issue := AnalyzeIssue{
          Severity: "error", Category: "file",
          Title: "Arquivo de mídia não encontrado",
          Description: "O arquivo referenciado não existe no disco. Isso pode acontecer se o disco externo foi desconectado ou se o mergerfs não montou corretamente.",
          MediaName: extractMediaName(context), RelevantLogs: context,
          Suggestion: "Verifique se o disco está montado (lsblk) e se o mergerfs está funcionando. Um 'systemctl restart mergerfs' pode resolver se o disco está conectado mas não montado.",
        }
        enrichIssue(&issue, i)
        response.Issues = append(response.Issues, issue)
      }
    }

    // ── I/O errors ──
    if reIOError.MatchString(trimmed) {
      key := "io_error_" + fmt.Sprintf("%d", i/10)
      if !seenIssues[key] {
        seenIssues[key] = true
        context := getContext(lines, i, 2)
        issue := AnalyzeIssue{
          Severity: "error", Category: "io",
          Title: "Erro de I/O ao ler mídia",
          Description: "Um erro de leitura de disco foi detectado. Isso indica problemas com o disco ou a conexão USB/SATA.",
          MediaName: extractMediaName(context), RelevantLogs: context,
          Suggestion: "Verifique a saúde do disco com 'smartctl' ou 'dmesg | tail -50'. Discos USB podem ter problemas de energia — tente reconectar ou usar um hub USB com alimentação própria.",
        }
        enrichIssue(&issue, i)
        response.Issues = append(response.Issues, issue)
      }
    }

    // ── Subtitle extraction errors ──
    if reSubtitleErr.MatchString(trimmed) {
      key := "subtitle_err_" + fmt.Sprintf("%d", i/10)
      if !seenIssues[key] {
        seenIssues[key] = true
        context := getContext(lines, i, 2)
        issue := AnalyzeIssue{
          Severity: "warning", Category: "subtitle",
          Title: "Problema com legendas",
          Description: "O Jellyfin encontrou um erro ao processar legendas. Legendas embutidas (ASS/SSA) podem forçar transcoding de vídeo, causando lentidão ou tela preta.",
          MediaName: extractMediaName(context), RelevantLogs: context,
          Suggestion: "Nas configurações de legenda do Jellyfin, mude de 'Burn-in' para 'Delivery' (entrega direta). Legendas SRT externas são mais leves que ASS embutido.",
        }
        enrichIssue(&issue, i)
        response.Issues = append(response.Issues, issue)
      }
    }

    // ── Playback sessions ──
    if rePlaybackStart.MatchString(trimmed) {
      session := AnalyzeSession{
        Timestamp: extractTimestamp(trimmed),
      }

      wideCtx := getContext(lines, i, 10)
      wideStr := strings.Join(wideCtx, " ")
      context := getContext(lines, i, 5)
      contextStr := strings.Join(context, " ")

      // Extract play method
      if pm := rePlayMethod.FindStringSubmatch(contextStr); pm != nil {
        session.PlayMethod = pm[1]
      } else if reTranscode.MatchString(contextStr) {
        session.PlayMethod = "Transcode"
      } else if reDirectStream.MatchString(contextStr) {
        session.PlayMethod = "DirectStream"
      }

      // Extract client info (wider context for better detection)
      if cl := reJfClient.FindStringSubmatch(wideStr); cl != nil {
        session.Client = strings.TrimSpace(cl[1])
      } else if cl := reClient.FindStringSubmatch(wideStr); cl != nil {
        session.Client = strings.TrimSpace(cl[1])
      }
      if dv := reJfDevice.FindStringSubmatch(wideStr); dv != nil {
        session.Device = strings.TrimSpace(dv[1])
      } else if dv := reDevice.FindStringSubmatch(wideStr); dv != nil {
        session.Device = strings.TrimSpace(dv[1])
      }

      session.ClientType = classifyClient(session.Client, session.Device)

      session.MediaName = extractMediaName(context)
      if session.MediaName == "" {
        session.MediaName = "(mídia não identificada)"
      }

      response.Sessions = append(response.Sessions, session)
    }

    // ── Transcoding decision (even if no error, inform the user) ──
    if reTranscode.MatchString(trimmed) && !reCodecUnsupported.MatchString(trimmed) {
      key := "transcode_info_" + fmt.Sprintf("%d", i/15)
      if !seenIssues[key] {
        seenIssues[key] = true
        context := getContext(lines, i, 2)
        codecMatches := reCodecInfo.FindAllString(strings.Join(context, " "), -1)
        codecStr := strings.Join(unique(codecMatches), ", ")
        desc := "O Jellyfin decidiu transcodificar o vídeo ao invés de fazer stream direto."
        if codecStr != "" {
          desc = fmt.Sprintf("O Jellyfin decidiu transcodificar o vídeo (codecs detectados: %s) ao invés de fazer stream direto.", strings.ToUpper(codecStr))
        }
        issue := AnalyzeIssue{
          Severity:     "info",
          Category:     "transcoding",
          Title:        "Transcoding ativado para esta mídia",
          Description:  desc,
          MediaName:    extractMediaName(context),
          RelevantLogs: context,
          Suggestion:   "Transcoding no Rock 3A é pesado e pode causar buffering. Se possível, use arquivos com codec H.264 (compatível com a maioria das TVs) para evitar transcoding.",
        }
        enrichIssue(&issue, i)
        response.Issues = append(response.Issues, issue)
      }
    }
  }

  w.Header().Set("Content-Type", "application/json")
  json.NewEncoder(w).Encode(response)
}

// ── Helpers ──

// getContext returns surrounding lines for context
func getContext(lines []string, index, radius int) []string {
  start := index - radius
  if start < 0 {
    start = 0
  }
  end := index + radius + 1
  if end > len(lines) {
    end = len(lines)
  }
  var result []string
  for _, l := range lines[start:end] {
    trimmed := strings.TrimSpace(l)
    if trimmed != "" {
      result = append(result, trimmed)
    }
  }
  return result
}

// extractMediaName tries to find a media/file name from log lines
func extractMediaName(lines []string) string {
  combined := strings.Join(lines, " ")

  if m := reItemName.FindStringSubmatch(combined); m != nil {
    name := strings.TrimSpace(m[1])
    if len(name) > 3 && len(name) < 200 {
      return name
    }
  }

  if m := reMediaItem.FindStringSubmatch(combined); m != nil {
    name := strings.TrimSpace(m[1])
    if len(name) > 3 && len(name) < 200 {
      return name
    }
  }

  return ""
}

// extractTimestamp extracts timestamp from a journal log line
func extractTimestamp(line string) string {
  if m := reTimestamp.FindStringSubmatch(line); m != nil {
    return m[1]
  }
  return ""
}

// unique deduplicates a string slice
func unique(input []string) []string {
  seen := make(map[string]bool)
  var result []string
  for _, s := range input {
    lower := strings.ToLower(s)
    if !seen[lower] {
      seen[lower] = true
      result = append(result, s)
    }
  }
  return result
}

// extractClientInfo tries to find client app name from surrounding log lines
func extractClientInfo(lines []string) string {
  combined := strings.Join(lines, " ")
  if m := reJfClient.FindStringSubmatch(combined); m != nil {
    return strings.TrimSpace(m[1])
  }
  if m := reClient.FindStringSubmatch(combined); m != nil {
    return strings.TrimSpace(m[1])
  }
  return ""
}

// extractDeviceInfo tries to find device name from surrounding log lines
func extractDeviceInfo(lines []string) string {
  combined := strings.Join(lines, " ")
  if m := reJfDevice.FindStringSubmatch(combined); m != nil {
    return strings.TrimSpace(m[1])
  }
  if m := reDevice.FindStringSubmatch(combined); m != nil {
    return strings.TrimSpace(m[1])
  }
  return ""
}

// classifyClient determines the client type (TV, Browser, Mobile, Desktop)
func classifyClient(client, device string) string {
  c := strings.ToLower(client + " " + device)

  // TV clients
  tvKeywords := []string{"android tv", "androidtv", "fire tv", "firetv", "chromecast",
    "tizen", "webos", "roku", "apple tv", "appletv", "smart tv", "smarttv",
    "tcl", "samsung", "lg ", "sony", "hisense", "philips", "vizio",
    "jellyfin androidtv", "swiftfin tvos", "infuse", "findroid",
    "shield", "nvidia shield", "mi box", "mecool", "tv box"}
  for _, kw := range tvKeywords {
    if strings.Contains(c, kw) {
      return "📺 TV"
    }
  }

  // Browser clients
  browserKeywords := []string{"jellyfin web", "chrome", "firefox", "safari",
    "edge", "mozilla", "opera", "brave", "vivaldi", "web browser",
    "chromium"}
  for _, kw := range browserKeywords {
    if strings.Contains(c, kw) {
      return "🌐 Browser"
    }
  }

  // Mobile clients
  mobileKeywords := []string{"jellyfin mobile", "jellyfin android", "jellyfin ios",
    "swiftfin", "iphone", "ipad", "android", "pixel", "galaxy",
    "xiaomi", "oneplus", "huawei", "motorola", "findroid"}
  for _, kw := range mobileKeywords {
    if strings.Contains(c, kw) {
      return "📱 Mobile"
    }
  }

  // Desktop clients
  desktopKeywords := []string{"jellyfin media player", "jmp", "desktop",
    "windows", "macos", "linux"}
  for _, kw := range desktopKeywords {
    if strings.Contains(c, kw) {
      return "💻 Desktop"
    }
  }

  if client != "" || device != "" {
    return "❓ Desconhecido"
  }
  return ""
}
