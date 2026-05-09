package config

import "os"

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

func init() {
	Config = APIConfig{
		RadarrURL:   getEnv("RADARR_URL", "http://localhost:7878"),
		RadarrKey:   getEnv("RADARR_API_KEY", ""),
		SonarrURL:   getEnv("SONARR_URL", "http://localhost:8989"),
		SonarrKey:   getEnv("SONARR_API_KEY", ""),
		JellyfinURL: getEnv("JELLYFIN_URL", "http://localhost:8096"),
		JellyfinKey: getEnv("JELLYFIN_API_KEY", ""),
	}
}
