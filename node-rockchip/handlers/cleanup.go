package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"rockchip-node/config"
	"time"
)

type CleanupItem struct {
	ID         string    `json:"id"`
	Type       string    `json:"type"` // "movie" or "series"
	Title      string    `json:"title"`
	SizeGB     float64   `json:"size_gb"`
	AddedDate  time.Time `json:"added_date"`
	IsWatched  bool      `json:"is_watched"`
	WatchedAt  time.Time `json:"watched_at,omitempty"`
	HoldDays        int       `json:"hold_days"`
	Suggestion      string    `json:"suggestion"` // "keep" or "delete"
	Reason          string    `json:"reason"`
	TotalEpisodes   int       `json:"total_episodes,omitempty"`
	WatchedEpisodes int       `json:"watched_episodes,omitempty"`
}

func CleanupHandler(w http.ResponseWriter, r *http.Request) {
	movies, err := getRadarrMovies()
	if err != nil {
		http.Error(w, fmt.Sprintf("Error fetching Radarr movies: %v", err), http.StatusInternalServerError)
		return
	}

	series, err := getSonarrSeries()
	if err != nil {
		http.Error(w, fmt.Sprintf("Error fetching Sonarr series: %v", err), http.StatusInternalServerError)
		return
	}

	// Fetch watched status from Jellyfin
	watchedPaths, err := getJellyfinWatchedItems()
	if err != nil {
		// Log error but continue with empty watched info if needed
		fmt.Printf("Jellyfin error: %v\n", err)
	}

	var results []CleanupItem
	now := time.Now()

	// Process Movies
	for _, m := range movies {
		item := CleanupItem{
			ID:        fmt.Sprintf("radarr_%v", m["id"]),
			Type:      "movie",
			Title:     m["title"].(string),
			AddedDate: parseISO8601(m["added"].(string)),
		}

		if hasFile, _ := m["hasFile"].(bool); !hasFile {
			item.SizeGB = 0
			item.Reason = "Arquivo não encontrado no Radarr"
		} else if movieFile, ok := m["movieFile"].(map[string]interface{}); ok {
			if size, ok := movieFile["size"].(float64); ok {
				item.SizeGB = size / (1024 * 1024 * 1024)
			}
			if path, ok := movieFile["path"].(string); ok {
				if watchedAt, played := watchedPaths[path]; played {
					item.IsWatched = true
					item.WatchedAt = watchedAt
				}
			}
		}

		if item.IsWatched && !item.WatchedAt.IsZero() {
			item.HoldDays = int(now.Sub(item.WatchedAt).Hours() / 24)
		} else {
			item.HoldDays = int(now.Sub(item.AddedDate).Hours() / 24)
		}
		
		// Logic: Watched > 60 days OR Unwatched > 90 days
		if item.IsWatched && item.HoldDays > 60 {
			item.Suggestion = "delete"
			item.Reason = "Assistido e mantido por > 60 dias"
		} else if !item.IsWatched && item.HoldDays > 90 {
			item.Suggestion = "delete"
			item.Reason = "Não assistido e mantido por > 90 dias"
		} else {
			item.Suggestion = "keep"
			item.Reason = "Dentro do prazo de retenção"
		}

		results = append(results, item)
	}

	// Process Series (Simplified to High Level Series)
	for _, s := range series {
		item := CleanupItem{
			ID:        fmt.Sprintf("sonarr_%v", s["id"]),
			Type:      "series",
			Title:     s["title"].(string),
			AddedDate: parseISO8601(s["added"].(string)),
		}

		if stats, ok := s["statistics"].(map[string]interface{}); ok {
			if size, ok := stats["sizeOnDisk"].(float64); ok {
				item.SizeGB = size / (1024 * 1024 * 1024)
				if item.SizeGB == 0 {
					item.Reason = "Nenhum arquivo baixado para esta série"
				}
			}
			if count, ok := stats["episodeCount"].(float64); ok {
				item.TotalEpisodes = int(count)
			}
		}
		
		// For series, we check if the series path contains any watched file
		if seriesPath, ok := s["path"].(string); ok && seriesPath != "" {
			var latestWatched time.Time
			for watchedPath, watchedAt := range watchedPaths {
				if len(watchedPath) >= len(seriesPath) && watchedPath[:len(seriesPath)] == seriesPath {
					item.WatchedEpisodes++
					if watchedAt.After(latestWatched) {
						latestWatched = watchedAt
					}
				}
			}
			if item.WatchedEpisodes > 0 {
				item.IsWatched = true
				item.WatchedAt = latestWatched
			}
		}
		
		if item.IsWatched && !item.WatchedAt.IsZero() {
			item.HoldDays = int(now.Sub(item.WatchedAt).Hours() / 24)
		} else {
			item.HoldDays = int(now.Sub(item.AddedDate).Hours() / 24)
		}
		
		// Logic for Series
		isCompleted := item.TotalEpisodes > 0 && item.WatchedEpisodes >= item.TotalEpisodes
		
		if isCompleted && item.HoldDays > 60 {
			item.Suggestion = "delete"
			item.Reason = "Série completa assistida e mantida por > 60 dias"
		} else if !item.IsWatched && item.HoldDays > 90 {
			item.Suggestion = "delete"
			item.Reason = "Não iniciada e mantida por > 90 dias"
		} else if item.IsWatched && !isCompleted {
			item.Suggestion = "keep"
			item.Reason = "Série em andamento"
		} else {
			item.Suggestion = "keep"
			item.Reason = "Dentro do prazo de retenção"
		}

		results = append(results, item)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

func getRadarrMovies() ([]map[string]interface{}, error) {
	url := fmt.Sprintf("%s/api/v3/movie?apiKey=%s", config.Config.RadarrURL, config.Config.RadarrKey)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var movies []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&movies); err != nil {
		return nil, err
	}
	return movies, nil
}

func getSonarrSeries() ([]map[string]interface{}, error) {
	url := fmt.Sprintf("%s/api/v3/series?apiKey=%s", config.Config.SonarrURL, config.Config.SonarrKey)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var series []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&series); err != nil {
		return nil, err
	}
	return series, nil
}

func getJellyfinWatchedItems() (map[string]time.Time, error) {
	// First, fetch the system users to find a valid one
	usersUrl := fmt.Sprintf("%s/Users?api_key=%s", config.Config.JellyfinURL, config.Config.JellyfinKey)
	resp, err := http.Get(usersUrl)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var users []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&users); err != nil || len(users) == 0 {
		return nil, fmt.Errorf("no users found in Jellyfin")
	}

	userId := ""
	for _, user := range users {
		if name, ok := user["Name"].(string); ok && name == "olucas" {
			userId = user["Id"].(string)
			break
		}
	}

	if userId == "" {
		// Fallback to first user if olucas is not found, or return error?
		// The user specifically asked for 'olucas', so let's error if not found to be explicit
		return nil, fmt.Errorf("user 'olucas' not found in Jellyfin")
	}

	// Now fetch items for that user with Recursive=true and IsPlayed=true
	// We need UserData to get LastPlayedDate
	itemsUrl := fmt.Sprintf("%s/Users/%s/Items?api_key=%s&Recursive=true&Fields=Path,UserData&IsPlayed=true", config.Config.JellyfinURL, userId, config.Config.JellyfinKey)
	resp, err = http.Get(itemsUrl)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var data struct {
		Items []struct {
			Path     string `json:"Path"`
			UserData struct {
				LastPlayedDate time.Time `json:"LastPlayedDate"`
			} `json:"UserData"`
		} `json:"Items"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}

	watched := make(map[string]time.Time)
	for _, item := range data.Items {
		if item.Path != "" {
			watched[item.Path] = item.UserData.LastPlayedDate
		}
	}
	return watched, nil
}

func parseISO8601(ts string) time.Time {
	t, _ := time.Parse(time.RFC3339, ts)
	return t
}
