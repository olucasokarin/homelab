package config

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"
)

type Config struct {
	ActivePath string `json:"active_path"` // "filmes", "series", "radarr" or "sonarr"
	SubFolder  string `json:"sub_folder"`  // e.g., "Breaking Bad S01"

	mu sync.RWMutex `json:"-"`
}

var (
	GlobalConfig *Config
	configPath   = "config.json"

	BasePaths = map[string]string{
		"filmes": "/mnt/storage/filmes",
		"series": "/mnt/storage/series",
		"radarr": "/mnt/storage/downloads/radarr",
		"sonarr": "/mnt/storage/downloads/sonarr",
	}
	IncompletePath = "/mnt/storage/downloads/incompletos"
)

func Load() {
	GlobalConfig = &Config{
		ActivePath: "series", // default
		SubFolder:  "",
	}

	data, err := os.ReadFile(configPath)
	if err == nil {
		if err := json.Unmarshal(data, GlobalConfig); err != nil {
			log.Printf("Error unmarshaling config: %v\n", err)
		}
	} else {
		log.Printf("No config.json found, using defaults.\n")
	}

	// Create required directories
	os.MkdirAll(IncompletePath, 0755)
	for _, path := range BasePaths {
		os.MkdirAll(path, 0755)
	}
}

func (c *Config) Save() {
	c.mu.RLock()
	defer c.mu.RUnlock()

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		log.Printf("Error saving config: %v\n", err)
		return
	}
	os.WriteFile(configPath, data, 0644)
}

func (c *Config) SetActivePath(path string) bool {
	if _, ok := BasePaths[path]; !ok {
		return false
	}

	c.mu.Lock()
	c.ActivePath = path
	c.SubFolder = "" // Reset subfolder on path change
	c.mu.Unlock()

	c.Save()
	return true
}

func (c *Config) SetSubFolder(folder string) {
	c.mu.Lock()
	c.SubFolder = folder
	c.mu.Unlock()
	c.Save()
}

func (c *Config) GetFinalDir() string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	base := BasePaths[c.ActivePath]
	if c.SubFolder != "" {
		finalPath := filepath.Join(base, c.SubFolder)
		os.MkdirAll(finalPath, 0755)
		return finalPath
	}
	return base
}

func (c *Config) GetStatus() (string, string) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ActivePath, c.SubFolder
}
