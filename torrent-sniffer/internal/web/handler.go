package web

import (
	"encoding/json"
	"net/http"
	"net/url"

	"torrent-sniffer/internal/domain"
	"torrent-sniffer/internal/sniffer"
)

type Handler struct {
	snifferSvc *sniffer.Service
}

func NewHandler(snifferSvc *sniffer.Service) *Handler {
	return &Handler{snifferSvc: snifferSvc}
}

func (h *Handler) History(w http.ResponseWriter, r *http.Request) {
	history, err := h.snifferSvc.GetHistory(50)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(history)
}

func (h *Handler) Queue(w http.ResponseWriter, r *http.Request) {
	tasks := h.snifferSvc.GetActiveTasks()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(tasks)
}

func (h *Handler) Cancel(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "Task ID is required", http.StatusBadRequest)
		return
	}
	h.snifferSvc.CancelTask(id)
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"cancelling"}`))
}

func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "ID is required", http.StatusBadRequest)
		return
	}
	if err := h.snifferSvc.DeleteHistory(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) Flush(w http.ResponseWriter, r *http.Request) {
	if err := h.snifferSvc.FlushHistory(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) SaveNotes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		ID    string `json:"id"`
		Notes string `json:"notes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	if req.ID == "" {
		http.Error(w, "ID is required", http.StatusBadRequest)
		return
	}
	if err := h.snifferSvc.UpdateNotes(req.ID, req.Notes); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (h *Handler) Sniff(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req domain.SniffRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	if req.MagnetURI == "" {
		http.Error(w, "magnet field is required", http.StatusBadRequest)
		return
	}

	if unescaped, err := url.QueryUnescape(req.MagnetURI); err == nil {
		req.MagnetURI = unescaped
	}

	res, err := h.snifferSvc.Process(r.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(res)
}

func Health(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}
