package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"strings"

	"rockchip-node/metrics"
	"rockchip-node/models"
)

func Cors(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		h(w, r)
	}
}

func MetricsHandler(w http.ResponseWriter, r *http.Request) {
	usage, cores := metrics.CPUUsage()
	topCPU, topMem, topIO, self := metrics.GetTopProcesses()

	var botProc models.Process
	var snifferProc models.Process
	all, _ := metrics.GetProcessInfo()
	for _, p := range all {
		if strings.Contains(p.Name, "telebot") || strings.Contains(p.Name, "rockchip-bot") {
			botProc = p
		}
		if strings.Contains(p.Name, "torrent-sniffer") {
			snifferProc = p
		}
	}

	m := models.Metrics{
		Version:   "v9-mergefs",
		Timestamp: models.GetTimestamp(),
		Thermals:  metrics.ReadThermals(),
		CPU: models.CPUStats{
			UsagePercent: usage,
			CoreCount:    cores,
		},
		GPU:    metrics.ReadGPU(),
		Disks:  metrics.ReadDisks(),
		Memory: metrics.ReadMem(),
		TopCPU: topCPU,
		TopMem: topMem,
		TopIO:  topIO,
		Self:   self,
		Bot:    botProc,
		Sniffer: snifferProc,
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(m)
}

func HealthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, `{"status":"ok"}`)
}

func LogsHandler(w http.ResponseWriter, r *http.Request) {
	service := r.URL.Query().Get("service")
	lines := r.URL.Query().Get("lines")
	if lines == "" {
		lines = "100"
	}

	validServices := map[string]bool{
		"jellyfin":         true,
		"radarr":           true,
		"sonarr":           true,
		"bazarr":           true,
		"prowlarr":         true,
		"transmission":     true,
		"qbittorrent":      true,
		"qbittorrent-nox":  true,
		"rockchip-bot":     true,
		"rockchip-node":    true,
		"telegram-bot-api": true,
		"torrent-sniffer":  true,
	}
	if !validServices[service] {
		http.Error(w, "Invalid service", http.StatusBadRequest)
		return
	}

	var cmd *exec.Cmd
	if service == "jellyfin" || service == "transmission" || service == "qbittorrent" || service == "qbittorrent-nox" || service == "rockchip-bot" || service == "rockchip-node" || service == "torrent-sniffer" {
		unit := service
		if service == "qbittorrent" || service == "qbittorrent-nox" {
			unit = "qbittorrent-nox"
		}
		cmd = exec.Command("journalctl", "-u", unit, "-n", lines, "--no-pager")
	} else {
		cmd = exec.Command("docker", "compose", "-f", "/home/olucas/arr-stack/docker-compose.yml", "logs", "--tail", lines, service)
	}

	out, err := cmd.CombinedOutput()
	if err != nil && service != "jellyfin" && service != "transmission" && service != "qbittorrent" && service != "rockchip-bot" && service != "rockchip-node" && service != "torrent-sniffer" {
		cmd = exec.Command("docker-compose", "-f", "/home/olucas/arr-stack/docker-compose.yml", "logs", "--tail", lines, service)
		out, err = cmd.CombinedOutput()
	}

	if err != nil {
		out = []byte(fmt.Sprintf("Failed to get logs for %s.\nError: %v\nOutput: %s", service, err, string(out)))
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write(out)
}

func RebootHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	log.Println("Reboot requested via API")
	cmd := exec.Command("sudo", "reboot")
	err := cmd.Start()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to initiate reboot: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, `{"status":"rebooting"}`)
}

func RestartHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	service := r.URL.Query().Get("service")
	if service == "" {
		http.Error(w, "Service name is required", http.StatusBadRequest)
		return
	}

	// Special group: restart the entire arr stack
	if service == "arr-stack" {
		log.Println("Restarting entire arr-stack via docker compose...")
		cmd := exec.Command("docker", "compose", "-f", "/home/olucas/arr-stack/docker-compose.yml", "restart")
		out, err := cmd.CombinedOutput()
		if err != nil {
			cmd = exec.Command("docker-compose", "-f", "/home/olucas/arr-stack/docker-compose.yml", "restart")
			out, err = cmd.CombinedOutput()
		}
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to restart arr-stack: %v\nOutput: %s", err, string(out)), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"status":"restarted", "service":"arr-stack"}`)
		return
	}

	validServices := map[string]bool{
		"jellyfin":         true,
		"radarr":           true,
		"sonarr":           true,
		"bazarr":           true,
		"prowlarr":         true,
		"transmission":     true,
		"qbittorrent":      true,
		"qbittorrent-nox":  true,
		"rockchip-bot":     true,
		"rockchip-node":    true,
		"telegram-bot-api": true,
		"torrent-sniffer":  true,
	}

	if !validServices[service] {
		http.Error(w, "Invalid service", http.StatusBadRequest)
		return
	}

	var cmd *exec.Cmd
	if service == "jellyfin" || service == "transmission" || service == "qbittorrent" || service == "qbittorrent-nox" || service == "rockchip-bot" || service == "rockchip-node" || service == "torrent-sniffer" {
		unit := service
		if service == "qbittorrent" || service == "qbittorrent-nox" {
			unit = "qbittorrent-nox"
		} else if service == "transmission" {
			unit = "transmission-daemon"
		}
		log.Printf("Restarting systemd service: %s", unit)
		cmd = exec.Command("sudo", "systemctl", "restart", unit)
	} else {
		log.Printf("Restarting docker compose service: %s", service)
		cmd = exec.Command("docker", "compose", "-f", "/home/olucas/arr-stack/docker-compose.yml", "restart", service)
	}

	out, err := cmd.CombinedOutput()
	if err != nil && service != "jellyfin" && service != "transmission" && service != "qbittorrent" && service != "rockchip-bot" && service != "rockchip-node" && service != "torrent-sniffer" {
		cmd = exec.Command("docker-compose", "-f", "/home/olucas/arr-stack/docker-compose.yml", "restart", service)
		out, err = cmd.CombinedOutput()
	}

	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to restart %s: %v\nOutput: %s", service, err, string(out)), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"status":"restarted", "service":"%s"}`, service)
}

func StopHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	service := r.URL.Query().Get("service")
	if service == "" {
		http.Error(w, "Service name is required", http.StatusBadRequest)
		return
	}

	// Special group: stop the entire arr stack (all docker compose services)
	if service == "arr-stack" {
		log.Println("Stopping entire arr-stack via docker compose...")
		cmd := exec.Command("docker", "compose", "-f", "/home/olucas/arr-stack/docker-compose.yml", "stop")
		out, err := cmd.CombinedOutput()
		if err != nil {
			cmd = exec.Command("docker-compose", "-f", "/home/olucas/arr-stack/docker-compose.yml", "stop")
			out, err = cmd.CombinedOutput()
		}
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to stop arr-stack: %v\nOutput: %s", err, string(out)), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"status":"stopped", "service":"arr-stack"}`)
		return
	}

	validServices := map[string]bool{
		"jellyfin":         true,
		"transmission":     true,
		"qbittorrent":      true,
		"qbittorrent-nox":  true,
		"rockchip-bot":     true,
		"torrent-sniffer":  true,
		"radarr":           true,
		"sonarr":           true,
		"bazarr":           true,
		"prowlarr":         true,
		"telegram-bot-api": true,
	}

	if !validServices[service] {
		http.Error(w, "Invalid service", http.StatusBadRequest)
		return
	}

	var cmd *exec.Cmd
	if service == "jellyfin" || service == "transmission" || service == "qbittorrent" || service == "qbittorrent-nox" || service == "rockchip-bot" || service == "torrent-sniffer" {
		unit := service
		if service == "qbittorrent" || service == "qbittorrent-nox" {
			unit = "qbittorrent-nox"
		} else if service == "transmission" {
			unit = "transmission-daemon"
		}
		log.Printf("Stopping systemd service: %s", unit)
		cmd = exec.Command("sudo", "systemctl", "stop", unit)
	} else {
		log.Printf("Stopping docker compose service: %s", service)
		cmd = exec.Command("docker", "compose", "-f", "/home/olucas/arr-stack/docker-compose.yml", "stop", service)
	}

	out, err := cmd.CombinedOutput()
	if err != nil && service != "jellyfin" && service != "transmission" && service != "qbittorrent" && service != "rockchip-bot" && service != "torrent-sniffer" {
		cmd = exec.Command("docker-compose", "-f", "/home/olucas/arr-stack/docker-compose.yml", "stop", service)
		out, err = cmd.CombinedOutput()
	}

	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to stop %s: %v\nOutput: %s", service, err, string(out)), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"status":"stopped", "service":"%s"}`, service)
}

func StartHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	service := r.URL.Query().Get("service")
	if service == "" {
		http.Error(w, "Service name is required", http.StatusBadRequest)
		return
	}

	// Special group: start the entire arr stack (all docker compose services)
	if service == "arr-stack" {
		log.Println("Starting entire arr-stack via docker compose...")
		cmd := exec.Command("docker", "compose", "-f", "/home/olucas/arr-stack/docker-compose.yml", "start")
		out, err := cmd.CombinedOutput()
		if err != nil {
			cmd = exec.Command("docker-compose", "-f", "/home/olucas/arr-stack/docker-compose.yml", "start")
			out, err = cmd.CombinedOutput()
		}
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to start arr-stack: %v\nOutput: %s", err, string(out)), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"status":"started", "service":"arr-stack"}`)
		return
	}

	validServices := map[string]bool{
		"jellyfin":         true,
		"transmission":     true,
		"qbittorrent":      true,
		"qbittorrent-nox":  true,
		"rockchip-bot":     true,
		"torrent-sniffer":  true,
		"radarr":           true,
		"sonarr":           true,
		"bazarr":           true,
		"prowlarr":         true,
		"telegram-bot-api": true,
	}

	if !validServices[service] {
		http.Error(w, "Invalid service", http.StatusBadRequest)
		return
	}

	var cmd *exec.Cmd
	if service == "jellyfin" || service == "transmission" || service == "qbittorrent" || service == "qbittorrent-nox" || service == "rockchip-bot" || service == "torrent-sniffer" {
		unit := service
		if service == "qbittorrent" || service == "qbittorrent-nox" {
			unit = "qbittorrent-nox"
		} else if service == "transmission" {
			unit = "transmission-daemon"
		}
		log.Printf("Starting systemd service: %s", unit)
		cmd = exec.Command("sudo", "systemctl", "start", unit)
	} else {
		log.Printf("Starting docker compose service: %s", service)
		cmd = exec.Command("docker", "compose", "-f", "/home/olucas/arr-stack/docker-compose.yml", "start", service)
	}

	out, err := cmd.CombinedOutput()
	if err != nil && service != "jellyfin" && service != "transmission" && service != "qbittorrent" && service != "rockchip-bot" && service != "torrent-sniffer" {
		cmd = exec.Command("docker-compose", "-f", "/home/olucas/arr-stack/docker-compose.yml", "start", service)
		out, err = cmd.CombinedOutput()
	}

	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to start %s: %v\nOutput: %s", service, err, string(out)), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"status":"started", "service":"%s"}`, service)
}

func QbtPauseHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	log.Println("Pausing all qBittorrent torrents...")
	resp, err := http.Post("http://localhost:8080/api/v2/torrents/pause?hashes=all", "application/x-www-form-urlencoded", nil)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to pause torrents: %v", err), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, `{"status":"paused"}`)
}

func QbtResumeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	log.Println("Resuming all qBittorrent torrents...")
	resp, err := http.Post("http://localhost:8080/api/v2/torrents/resume?hashes=all", "application/x-www-form-urlencoded", nil)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to resume torrents: %v", err), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, `{"status":"resumed"}`)
}
