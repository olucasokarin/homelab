package config

import (
	"os"
	"strings"
)

type APIConfig struct {
	RadarrURL   string
	RadarrKey   string
	SonarrURL   string
	SonarrKey   string
	JellyfinURL string
	JellyfinKey string
}

var Config APIConfig

func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}

func loadDotEnv(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(line, "\ufeff"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		if len(val) >= 2 {
			if (val[0] == '"' && val[len(val)-1] == '"') || (val[0] == '\'' && val[len(val)-1] == '\'') {
				val = val[1 : len(val)-1]
			}
		}
		if key == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); !exists {
			os.Setenv(key, val)
		}
	}
}

func init() {
	loadDotEnv(".env")
	loadDotEnv("/home/olucas/.env")
	Config = APIConfig{
		RadarrURL:   getEnv("RADARR_URL", "http://localhost:7878"),
		RadarrKey:   getEnv("RADARR_API_KEY", ""),
		SonarrURL:   getEnv("SONARR_URL", "http://localhost:8989"),
		SonarrKey:   getEnv("SONARR_API_KEY", ""),
		JellyfinURL: getEnv("JELLYFIN_URL", "http://localhost:8096"),
		JellyfinKey: getEnv("JELLYFIN_API_KEY", ""),
	}
}
