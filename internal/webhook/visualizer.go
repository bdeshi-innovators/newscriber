package webhook

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"time"
)

//go:embed visualizer.html
var visualizerHTML []byte

// HandleVisualizer serves the embedded high-fidelity HTML visualizer dashboard.
func (h *Handler) HandleVisualizer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(visualizerHTML)
}

// HandleEpisodes serves a JSON API endpoint returning all episodes in the database,
// dynamically mapping each episode to the news articles that were summarized
// in the 2 hours preceding its creation.
func (h *Handler) HandleEpisodes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// We use COALESCE(NULLIF(..., 'undefined'), '') to handle 'undefined' placeholders safely.
	// We aggregate matching news_items articles as a JSON array using a subquery.
	query := `
		SELECT e.id, e.language, e.script, e.created_at, 
		       COALESCE(NULLIF(e.mp3_url, 'undefined'), '') as mp3,
		       COALESCE(NULLIF(e.ogg_url, 'undefined'), '') as ogg,
		       COALESCE(NULLIF(e.wav_url, 'undefined'), '') as wav,
		       COALESCE(e.title, '') as title,
		       COALESCE(e.description, '') as description,
		       COALESCE(e.status, 'published') as status,
		       (
		           SELECT COALESCE(json_agg(json_build_object(
		               'source', n.source_name,
		               'headline', n.headline,
		               'url', n.article_url
		           )), '[]'::json)
		           FROM news_items n
		           WHERE n.summarized_at BETWEEN e.created_at - INTERVAL '2 hours' AND e.created_at
		       ) as articles
		FROM episodes e
		ORDER BY e.created_at DESC;
	`
	
	rows, err := h.db.QueryContext(r.Context(), query)
	if err != nil {
		slog.Error("failed querying episodes for visualizer", "err", err)
		http.Error(w, "database query failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type EpisodeItem struct {
		ID             int64           `json:"id"`
		Language       string          `json:"language"`
		Script         string          `json:"script"`
		CreatedAt      string          `json:"created_at"`
		MP3URL         string          `json:"mp3_url"`
		OGGURL         string          `json:"ogg_url"`
		WAVURL         string          `json:"wav_url"`
		Title          string          `json:"title"`
		Description    string          `json:"description"`
		Status         string          `json:"status"`
		Articles       json.RawMessage `json:"articles"`
		DynamicSources []string        `json:"dynamic_sources"`
	}

	var episodes []EpisodeItem
	for rows.Next() {
		var ep EpisodeItem
		var rawArticles []byte
		var createdAt time.Time
		if err := rows.Scan(&ep.ID, &ep.Language, &ep.Script, &createdAt, &ep.MP3URL, &ep.OGGURL, &ep.WAVURL, &ep.Title, &ep.Description, &ep.Status, &rawArticles); err != nil {
			slog.Error("failed scanning episode for visualizer", "err", err)
			http.Error(w, "database scan failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		ep.CreatedAt = createdAt.Format(time.RFC3339)
		ep.Articles = json.RawMessage(rawArticles)

		// Parse articles to extract unique source names
		var parsedArticles []struct {
			Source string `json:"source"`
		}
		if err := json.Unmarshal(rawArticles, &parsedArticles); err == nil {
			sourceSet := make(map[string]struct{})
			for _, art := range parsedArticles {
				if art.Source != "" {
					sourceSet[art.Source] = struct{}{}
				}
			}
			ep.DynamicSources = make([]string, 0, len(sourceSet))
			for src := range sourceSet {
				ep.DynamicSources = append(ep.DynamicSources, src)
			}
		}

		episodes = append(episodes, ep)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(episodes)
}

// HandleTrigger forwards a drop trigger request to the n8n webhook node, passing options like language and auto_publish.
func (h *Handler) HandleTrigger(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read input JSON payload to proxy to n8n
	var bodyBytes []byte
	if r.Body != nil {
		var err error
		bodyBytes, err = io.ReadAll(r.Body)
		if err != nil {
			slog.Error("failed reading trigger request body", "err", err)
			http.Error(w, "failed reading body", http.StatusBadRequest)
			return
		}
	}

	// Trigger the n8n webhook node
	n8nURL := "http://n8n:5678/webhook/VoiceScribeV4/webhook/trigger-drop"
	
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, n8nURL, bytes.NewReader(bodyBytes))
	if err != nil {
		slog.Error("failed creating request to n8n", "err", err)
		http.Error(w, "failed creating n8n request: "+err.Error(), http.StatusInternalServerError)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		slog.Error("failed sending trigger to n8n", "err", err)
		http.Error(w, "failed sending trigger to n8n: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		slog.Error("n8n trigger returned non-200 status", "status", resp.Status)
		http.Error(w, "n8n trigger failed with status: "+resp.Status, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"success","message":"News pipeline triggered successfully in n8n!"}`))
}

// HandlePublish updates an episode's title, description, and status to 'published',
// and triggers an immediate refresh of the RSS feed for its language.
func (h *Handler) HandlePublish(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	type PublishRequest struct {
		ID          int64  `json:"id"`
		Title       string `json:"title"`
		Description string `json:"description"`
	}

	var req PublishRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		slog.Error("failed decoding publish request", "err", err)
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	if req.ID == 0 {
		http.Error(w, "missing episode id", http.StatusBadRequest)
		return
	}

	// 1. Fetch the language of the episode first to regenerate its RSS feed downstream
	var lang string
	err := h.db.QueryRowContext(r.Context(), "SELECT language FROM episodes WHERE id = $1", req.ID).Scan(&lang)
	if err != nil {
		slog.Error("failed finding episode for publishing", "id", req.ID, "err", err)
		http.Error(w, "episode not found: "+err.Error(), http.StatusNotFound)
		return
	}

	// 2. Update status to 'published' and set custom title & description
	_, err = h.db.ExecContext(r.Context(), 
		"UPDATE episodes SET title = $1, description = $2, status = 'published' WHERE id = $3",
		req.Title, req.Description, req.ID,
	)
	if err != nil {
		slog.Error("failed updating episode status to published", "id", req.ID, "err", err)
		http.Error(w, "database update failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// 3. Immediately regenerate and upload the RSS feed for this language
	if err := h.UpdateRSSFeed(r.Context(), lang); err != nil {
		slog.Error("failed regenerating RSS feed on publish", "lang", lang, "err", err)
	}

	slog.Info("successfully published draft episode", "id", req.ID, "lang", lang)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"success":true}`))
}
