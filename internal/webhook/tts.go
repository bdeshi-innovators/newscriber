package webhook

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"voicescribe-webhook/internal/tts"
)

type TTSPayload struct {
	Language string         `json:"language"`
	Script   []tts.Dialogue `json:"script"`
	Filename string         `json:"filename"`
}

func (h *Handler) HandleTTS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload TTSPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	if len(payload.Script) == 0 || payload.Filename == "" {
		http.Error(w, "script and filename are required", http.StatusBadRequest)
		return
	}

	if payload.Language == "" {
		payload.Language = "en"
	}

	results, err := h.tts.GenerateAndUpload(r.Context(), payload.Language, payload.Script, payload.Filename)
	if err != nil {
		http.Error(w, "tts failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// NEW: Auto-maintain the podcast RSS .xml feed in Cloudflare R2!
	if err := h.UpdateRSSFeed(r.Context(), payload.Language); err != nil {
		slog.Error("failed to automatically update RSS feed", "lang", payload.Language, "err", err)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(results)
}
