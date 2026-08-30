package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"rockchip-node/config"
	"rockchip-node/handlers"
)

func main() {
	port := "5000"
	if p := os.Getenv("PORT"); p != "" {
		port = p
	}

	mux := http.NewServeMux()
	
	// API Endpoints
	mux.HandleFunc("/health", handlers.Cors(handlers.HealthHandler))
	mux.HandleFunc("/metrics", handlers.Cors(handlers.MetricsHandler))
	mux.HandleFunc("/logs", handlers.Cors(handlers.LogsHandler))
	mux.HandleFunc("/reboot", handlers.Cors(handlers.RebootHandler))
	mux.HandleFunc("/restart", handlers.Cors(handlers.RestartHandler))
	mux.HandleFunc("/stop", handlers.Cors(handlers.StopHandler))
	mux.HandleFunc("/start", handlers.Cors(handlers.StartHandler))
	mux.HandleFunc("/cleanup", handlers.Cors(handlers.CleanupHandler))
	mux.HandleFunc("/api/movies", handlers.Cors(handlers.MoviesHandler))
	mux.HandleFunc("/api/episodes", handlers.Cors(handlers.EpisodesHandler))
	mux.HandleFunc("/api/mediainfo", handlers.Cors(handlers.MediaInfoHandler))
	mux.HandleFunc("/api/storage/analyze", handlers.Cors(handlers.StorageAnalyzeHandler))
	mux.HandleFunc("/api/jellyfin/analyze", handlers.Cors(handlers.JellyfinAnalyzeHandler))
	mux.HandleFunc("/api/ffmpeg/logs", handlers.Cors(handlers.FFmpegLogsHandler))
	mux.HandleFunc("/api/hdr/scan", handlers.Cors(handlers.HDRScanHandler))
	mux.HandleFunc("/api/iohealth", handlers.Cors(handlers.IOHealthHandler))
	mux.HandleFunc("/api/qbittorrent/pause", handlers.Cors(handlers.QbtPauseHandler))
	mux.HandleFunc("/api/qbittorrent/resume", handlers.Cors(handlers.QbtResumeHandler))

	// Static Dashboard
	fs := http.FileServer(http.Dir("static"))
	mux.Handle("/", handlers.Cors(func(w http.ResponseWriter, r *http.Request) {
		fs.ServeHTTP(w, r)
	}))

	addr := ":" + port
	fmt.Printf("\n🚀 Rockchip Node Server → http://0.0.0.0%s\n", addr)
	fmt.Println("  [GET]   /health   → API Status")
	fmt.Println("  [GET]   /metrics  → System Hardware JSON")
	fmt.Println("  [POST]  /restart  → Restart Services")
	fmt.Println("  [POST]  /reboot   → System Reboot")
	fmt.Println("  [VIEW]  /         → Dashboard UI")
	fmt.Printf("  [CFG]   jellyfin=%s radarr=%s sonarr=%s keys_set=%v\n",
		config.Config.JellyfinURL, config.Config.RadarrURL, config.Config.SonarrURL,
		config.Config.JellyfinKey != "" && config.Config.RadarrKey != "" && config.Config.SonarrKey != "")
	fmt.Println()
	
	log.Fatal(http.ListenAndServe(addr, mux))
}
