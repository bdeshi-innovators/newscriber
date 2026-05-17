package webhook

import (
	"context"
	"database/sql"
	"encoding/json"
	"encoding/xml"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"

	"voicescribe-webhook/internal/tts"
	"voicescribe-webhook/internal/users"
)

const (
	mockMP3URL = "https://newscriber.example/audio/latest.mp3"

	welcomeMenu = "Welcome to NewScriber. Choose your language: \n1️⃣ English \n2️⃣ Italiano \n3️⃣ বাংলা"

	confirmReplyTemplate = "You're set! Here is your latest update. Next drop in 2 hours. " + mockMP3URL

	reminderReply = "Got it. Your next NewScriber drop arrives in ~2 hours."
)

type Handler struct {
	repo                 users.UserRepository
	tts                  *tts.Client
	db                   *sql.DB
	twilioAccountSID     string
	twilioAuthToken      string
	twilioWhatsAppNumber string
	metaWhatsAppNumber   string
}

func NewHandler(repo users.UserRepository, ttsClient *tts.Client, db *sql.DB) *Handler {
	return &Handler{
		repo:                 repo,
		tts:                  ttsClient,
		db:                   db,
		twilioAccountSID:     os.Getenv("TWILIO_ACCOUNT_SID"),
		twilioAuthToken:      os.Getenv("TWILIO_AUTH_TOKEN"),
		twilioWhatsAppNumber: os.Getenv("TWILIO_WHATSAPP_NUMBER"),
		metaWhatsAppNumber:   os.Getenv("META_WHATSAPP_NUMBER"),
	}
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
		// Query DB for the actual latest episode URL of this language
		mp3URL, err := h.getLatestEpisodeMP3URL(r.Context(), lang)
		if err != nil {
			log.Printf("webhook: get latest episode MP3 from database: %v", err)
		}
		if mp3URL == "" {
			mp3URL = mockMP3URL
		}
		return "You're set! Here is your latest update. Next drop in 2 hours. " + mp3URL, nil
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

func (h *Handler) getLatestEpisodeMP3URL(ctx context.Context, lang string) (string, error) {
	if h.db == nil {
		return "", errors.New("database connection not available")
	}
	var mp3URL string
	query := `SELECT mp3_url FROM episodes WHERE language = $1 ORDER BY created_at DESC LIMIT 1`
	err := h.db.QueryRowContext(ctx, query, lang).Scan(&mp3URL)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil // No episodes generated yet
		}
		return "", err
	}
	return mp3URL, nil
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
