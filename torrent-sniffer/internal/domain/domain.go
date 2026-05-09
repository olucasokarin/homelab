package domain

import "encoding/json"

// SniffRequest represents an incoming sniff request
type SniffRequest struct {
	MagnetURI   string `json:"magnet"`
	HeadBytes   int64  `json:"head_bytes"`   // Bytes to read from start (default 10MB)
	TailBytes   int64  `json:"tail_bytes"`   // Bytes to read from end (default 0, set for moov-at-end)
	TimeoutSecs int    `json:"timeout_secs"` // Overall timeout (default 120)
}

// SniffResult wraps the probe JSON output
type SniffResult struct {
	TorrentName     string          `json:"torrent_name"`
	FileName        string          `json:"file_name"`
	FileSize        int64           `json:"file_size"`
	TorrentSize     int64           `json:"torrent_size"`
	Probe           json.RawMessage `json:"probe"`
	ProbeTool       string          `json:"probe_tool"` // "ffprobe" or "mediainfo"
	ElapsedMs       int64           `json:"elapsed_ms"`
	DownloadedBytes int64           `json:"downloaded_bytes"`
	FetchedFromTail bool            `json:"fetched_from_tail"`
	Seeds           int             `json:"seeds"`
	Peers           int             `json:"peers"`
	// Homelab is *homelab.Report (veredito BPP/HDR/áudio + llm_bundle); typed as any to keep domain free of internal imports.
	Homelab interface{} `json:"homelab,omitempty"`
}
