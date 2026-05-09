package bot

import (
	"encoding/json"
	"log"
	"net/http"
)

func StartAPI(port string) {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/telebot/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Content-Type", "application/json")
		if DlManager == nil {
			json.NewEncoder(w).Encode(map[string]string{"error": "Bot not initialized"})
			return
		}
		json.NewEncoder(w).Encode(DlManager.GetFullStatus())
	})

	mux.HandleFunc("/api/telebot/retry", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Content-Type", "application/json")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		if DlManager == nil {
			json.NewEncoder(w).Encode(map[string]string{"error": "Bot not initialized"})
			return
		}

		count := DlManager.RetryAll()
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "retrying",
			"count":  count,
		})
	})

	log.Printf("API Dashboard Server running on :%s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatalf("API Server failed: %v", err)
	}
}
