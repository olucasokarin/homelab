package handlers

import (
	"encoding/json"
	"net/http"
	"rockchip-node/metrics"
)

func IOHealthHandler(w http.ResponseWriter, r *http.Request) {
	health := metrics.GetIOHealth()

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(health); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
