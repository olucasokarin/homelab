package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

var apiClient = &http.Client{Timeout: 60 * time.Second}

func getJSON(url string, dest interface{}) error {
	resp, err := apiClient.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		preview := strings.TrimSpace(string(body))
		if preview == "" {
			preview = "(empty body)"
		}
		if len(preview) > 180 {
			preview = preview[:180] + "…"
		}
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, preview)
	}
	if len(body) == 0 {
		return fmt.Errorf("empty response body (HTTP %d) — API key/URL missing?", resp.StatusCode)
	}
	return json.Unmarshal(body, dest)
}
