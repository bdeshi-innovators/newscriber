package webhook

import (
	"encoding/json"
	"encoding/xml"
	"errors"
	"log"
	"net/http"
	"strings"

	"voicescribe-webhook/internal/users"
)

const (
	mockMP3URL = "https://voicescribe.example/audio/latest.mp3"

	welcomeMenu = "Welcome to VoiceScribe. Choose your language: \n1️⃣ English \n2️⃣ Italiano \n3️⃣ বাংলা"

	confirmReplyTemplate = "You're set! Here is your latest update. Next drop in 2 hours. " + mockMP3URL

	reminderReply = "Got it. Your next VoiceScribe drop arrives in ~2 hours."
)

type Handler struct {
	repo users.UserRepository
}

func NewHandler(repo users.UserRepository) *Handler {
	return &Handler{repo: repo}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	msg, err := ParseInbound(r)
	if err != nil {
		if errors.Is(err, ErrUnsupportedContentType) {
			http.Error(w, "unsupported content type", http.StatusBadRequest)
			return
		}
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}

	reply, err := h.decideReply(r, msg)
	if err != nil {
		log.Printf("webhook: decide reply for %s: %v", msg.Phone, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	writeReply(w, msg.ReplyMode, reply)
}

func (h *Handler) decideReply(r *http.Request, msg InboundMessage) (string, error) {
	body := strings.TrimSpace(msg.Body)

	if lang, ok := languageFromChoice(body); ok {
		if err := h.repo.UpsertUser(r.Context(), msg.Phone, lang); err != nil {
			return "", err
		}
		return confirmReplyTemplate, nil
	}

	user, err := h.repo.GetUser(r.Context(), msg.Phone)
	if err != nil {
		return "", err
	}

	if user == nil || strings.EqualFold(body, "get news") {
		return welcomeMenu, nil
	}

	return reminderReply, nil
}

func languageFromChoice(body string) (string, bool) {
	switch body {
	case "1":
		return "en", true
	case "2":
		return "it", true
	case "3":
		return "bn", true
	}
	return "", false
}

type twiMLResponse struct {
	XMLName xml.Name `xml:"Response"`
	Message string   `xml:"Message"`
}

func writeReply(w http.ResponseWriter, mode ReplyMode, text string) {
	switch mode {
	case ReplyTwiML:
		w.Header().Set("Content-Type", "text/xml; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		if err := xml.NewEncoder(w).Encode(twiMLResponse{Message: text}); err != nil {
			log.Printf("webhook: write twiml: %v", err)
		}
	case ReplyJSON:
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(map[string]string{"reply": text}); err != nil {
			log.Printf("webhook: write json: %v", err)
		}
	}
}
