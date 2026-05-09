package handlers

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
)

type StorageItem struct {
	Name  string `json:"name"`
	IsDir bool   `json:"is_dir"`
	Bytes int64  `json:"bytes"`
}

func calculateSize(path string) (int64, error) {
	var size int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size, err
}

func StorageAnalyzeHandler(w http.ResponseWriter, r *http.Request) {
	dirPath := r.URL.Query().Get("path")
	if dirPath == "" {
		http.Error(w, "Path is required", http.StatusBadRequest)
		return
	}

	entries, err := os.ReadDir(dirPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var items []StorageItem
	for _, entry := range entries {
		// skip lost+found and hidden files if desired, but let's just show everything
		fullPath := filepath.Join(dirPath, entry.Name())
		var size int64
		if entry.IsDir() {
			size, _ = calculateSize(fullPath)
		} else {
			info, err := entry.Info()
			if err == nil {
				size = info.Size()
			}
		}
		items = append(items, StorageItem{
			Name:  entry.Name(),
			IsDir: entry.IsDir(),
			Bytes: size,
		})
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].Bytes > items[j].Bytes
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(items)
}
