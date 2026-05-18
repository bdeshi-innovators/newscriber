package webhook

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"
	"unicode/utf8"
)

type EpisodeRow struct {
	ID            int64
	Script        string
	MP3URL        string
	CreatedAt     time.Time
	Title         string
	Description   string
	EpisodeNumber int
	Language      string
	SourceNames   []string
}

func (h *Handler) UpdateRSSFeed(ctx context.Context, lang string) error {
	slog.Info("updating RSS feed", "lang", lang)

	// 1. Fetch latest episodes for the language (limit to latest 50 to keep feed size lean)
	var rows *sql.Rows
	var err error
	if lang == "global" {
		rows, err = h.db.QueryContext(ctx, 
			"SELECT id, script, mp3_url, created_at, COALESCE(title, '') as title, COALESCE(description, '') as description, episode_number, language, source_names FROM episodes WHERE mp3_url IS NOT NULL AND status = 'published' ORDER BY created_at DESC LIMIT 100;",
		)
	} else {
		rows, err = h.db.QueryContext(ctx, 
			"SELECT id, script, mp3_url, created_at, COALESCE(title, '') as title, COALESCE(description, '') as description, episode_number, language, source_names FROM episodes WHERE language = $1 AND mp3_url IS NOT NULL AND status = 'published' ORDER BY created_at DESC LIMIT 50;", 
			lang,
		)
	}
	if err != nil {
		return fmt.Errorf("query episodes: %w", err)
	}
	defer rows.Close()

	var episodes []EpisodeRow
	for rows.Next() {
		var ep EpisodeRow
		var rawSources string
		if err := rows.Scan(&ep.ID, &ep.Script, &ep.MP3URL, &ep.CreatedAt, &ep.Title, &ep.Description, &ep.EpisodeNumber, &ep.Language, &rawSources); err != nil {
			return fmt.Errorf("scan episode: %w", err)
		}
		ep.SourceNames = parsePGArray(rawSources)
		episodes = append(episodes, ep)
	}

	// 2. Upload cover art image
	coverURL, err := h.getOrUploadCover(ctx, lang)
	if err != nil {
		slog.Error("failed uploading cover art", "lang", lang, "err", err)
		// Fallback placeholder
		coverURL = "https://images.unsplash.com/photo-1590602847861-f357a9332bbc?w=1400"
	}

	// 3. Generate XML string
	xmlString := h.buildRSSXML(lang, episodes, coverURL)

	// 4. Verify XML Syntax with Automatic Self-Healing
	var dummy struct{}
	if err := xml.Unmarshal([]byte(xmlString), &dummy); err != nil {
		slog.Warn("XML validation failed initially, triggering automatic reactive healing", "lang", lang, "err", err)
		
		// Level 1 Recovery: Brute-force clean all input texts to be strictly alphanumeric and space
		healedEpisodes := make([]EpisodeRow, len(episodes))
		for i, ep := range episodes {
			healedEpisodes[i] = ep
			healedEpisodes[i].Title = forceCleanAlphanumeric(ep.Title)
			healedEpisodes[i].Description = forceCleanAlphanumeric(ep.Description)
			healedEpisodes[i].Script = forceCleanAlphanumeric(ep.Script)
		}
		
		xmlString = h.buildRSSXML(lang, healedEpisodes, coverURL)
		
		// Verify again
		if err := xml.Unmarshal([]byte(xmlString), &dummy); err != nil {
			slog.Error("XML reactive healing level 1 failed, returning absolute minimal fallback feed", "lang", lang, "err", err)
			// Level 2 Recovery: Absolute minimal guaranteed-valid XML feed
			xmlString = h.buildMinimalFallbackRSS(lang, coverURL)
		}
	}

	// 5. Upload XML to Cloudflare R2
	filename := fmt.Sprintf("feed_%s.xml", lang)
	publicURL, err := h.tts.UploadXML(ctx, filename, []byte(xmlString))
	if err != nil {
		return fmt.Errorf("upload rss xml: %w", err)
	}

	slog.Info("RSS feed updated successfully", "lang", lang, "url", publicURL)
	return nil
}

func (h *Handler) buildRSSXML(lang string, episodes []EpisodeRow, coverURL string) string {
	langTitle := "English"
	if lang == "it" {
		langTitle = "Italiano"
	} else if lang == "bn" {
		langTitle = "বাংলা"
	} else if lang == "fr" {
		langTitle = "Français"
	} else if lang == "global" {
		langTitle = "Global"
	}

	title := fmt.Sprintf("NewScriber — %s Daily Briefing", langTitle)
	description := fmt.Sprintf("Your high-fidelity automated bilingual news briefing in %s.", langTitle)
	if lang == "global" {
		title = "NewScriber — Multilingual Global Briefing"
		description = "Your high-fidelity automated daily news briefing across all supported languages."
	}
	nowRFC := time.Now().Format(time.RFC1123Z)

	xml := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" 
    xmlns:itunes="http://www.itunes.com/dtds/podcast-1.0.dtd" 
    xmlns:content="http://purl.org/rss/1.0/modules/content/">
  <channel>
    <title>` + healText(title) + `</title>
    <description>` + healText(description) + `</description>
    <link>https://newscriber.unityflow.ai</link>
    <language>` + lang + `</language>
    <itunes:author>NewScriber AI</itunes:author>
    <itunes:summary>` + healText(description) + `</itunes:summary>
    <itunes:owner>
      <itunes:name>NewScriber AI</itunes:name>
      <itunes:email>podcast@unityflow.ai</itunes:email>
    </itunes:owner>
    <itunes:explicit>no</itunes:explicit>
    <itunes:category text="News"/>
    <itunes:image href="` + escapeXML(coverURL) + `"/>
    <image>
      <url>` + escapeXML(coverURL) + `</url>
      <title>` + healText(title) + `</title>
      <link>https://newscriber.unityflow.ai</link>
    </image>
    <pubDate>` + nowRFC + `</pubDate>
`

	for _, ep := range episodes {
		// Clean description: limit prose snippet for podcast directories
		snippet := ep.Description
		if snippet == "" {
			var scriptJSON []struct {
				Speaker string `json:"speaker"`
				Text    string `json:"text"`
			}
			var dialogueText string
			if err := json.Unmarshal([]byte(ep.Script), &scriptJSON); err == nil && len(scriptJSON) > 0 {
				var parts []string
				for _, d := range scriptJSON {
					parts = append(parts, fmt.Sprintf("%s: %s", d.Speaker, d.Text))
				}
				dialogueText = strings.Join(parts, " ")
			} else {
				dialogueText = ep.Script
			}
			snippet = h.generateEpisodeDescription(ep, dialogueText)
		} else {
			runes := []rune(snippet)
			if len(runes) > 400 {
				snippet = string(runes[:400]) + "..."
			}
		}
		
		pubDate := ep.CreatedAt.Format(time.RFC1123Z)
		
		itemLangTitle := "English"
		if ep.Language == "it" {
			itemLangTitle = "Italiano"
		} else if ep.Language == "bn" {
			itemLangTitle = "বাংলা"
		} else if ep.Language == "fr" {
			itemLangTitle = "Français"
		}

		epTitle := ep.Title
		if epTitle == "" {
			epTitle = fmt.Sprintf("NewScriber %s - Episode #%d", itemLangTitle, ep.EpisodeNumber)
		}

		xml += `    <item>
      <title>` + healText(epTitle) + `</title>
      <description><![CDATA[` + healCDATA(snippet) + `]]></description>
      <pubDate>` + pubDate + `</pubDate>
      <enclosure url="` + escapeXML(ep.MP3URL) + `" length="10000000" type="audio/mpeg"/>
      <guid isPermaLink="false">` + escapeXML(ep.MP3URL) + `</guid>
      <itunes:explicit>no</itunes:explicit>
    </item>
`
	}

	xml += `  </channel>
</rss>`

	return xml
}

func (h *Handler) getOrUploadCover(ctx context.Context, lang string) (string, error) {
	var srcName string
	switch lang {
	case "en", "global":
		srcName = "english.jpeg"
	case "it":
		srcName = "italian.jpeg"
	case "fr":
		srcName = "french.jpeg"
	case "bn":
		srcName = "bangla.jpeg"
	default:
		srcName = "english.jpeg"
	}

	path := fmt.Sprintf("/app/resources/podcast-covers/%s", srcName)
	data, err := os.ReadFile(path)
	if err != nil {
		path = fmt.Sprintf("resources/podcast-covers/%s", srcName)
		data, err = os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("read cover file: %w", err)
		}
	}

	destName := fmt.Sprintf("cover_%s.jpeg", lang)
	publicURL, err := h.tts.UploadAsset(ctx, destName, data, "image/jpeg")
	if err != nil {
		return "", fmt.Errorf("upload cover: %w", err)
	}

	return publicURL, nil
}

func sanitizeUTF8(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	var buf bytes.Buffer
	for _, r := range s {
		if r == utf8.RuneError {
			continue
		}
		buf.WriteRune(r)
	}
	return buf.String()
}

func escapeXML(s string) string {
	var buf bytes.Buffer
	if err := xml.EscapeText(&buf, []byte(s)); err != nil {
		r := strings.NewReplacer(
			"&", "&amp;",
			"<", "&lt;",
			">", "&gt;",
			`"`, "&quot;",
			`'`, "&apos;",
		)
		return r.Replace(s)
	}
	return buf.String()
}

func healText(s string) string {
	s = sanitizeUTF8(s)
	return escapeXML(s)
}

func healCDATA(s string) string {
	s = sanitizeUTF8(s)
	return strings.ReplaceAll(s, "]]>", "]]&gt;")
}

func forceCleanAlphanumeric(s string) string {
	s = sanitizeUTF8(s)
	var buf bytes.Buffer
	for _, r := range s {
		// Keep spaces, basic punctuation, alphanumeric, and standard international chars
		// but strip off any bracket/markup characters
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == ' ' || r == '.' || r == ',' || r == '-' || r == '!' || r == '?' || r > 127 {
			buf.WriteRune(r)
		}
	}
	return buf.String()
}

func (h *Handler) buildMinimalFallbackRSS(lang string, coverURL string) string {
	langTitle := "English"
	if lang == "it" {
		langTitle = "Italiano"
	} else if lang == "bn" {
		langTitle = "বাংলা"
	} else if lang == "fr" {
		langTitle = "Français"
	} else if lang == "global" {
		langTitle = "Global"
	}

	title := fmt.Sprintf("NewScriber — %s Daily Briefing (Healed Feed)", langTitle)
	description := fmt.Sprintf("Your high-fidelity automated daily news briefing in %s.", langTitle)
	nowRFC := time.Now().Format(time.RFC1123Z)

	return `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" 
    xmlns:itunes="http://www.itunes.com/dtds/podcast-1.0.dtd" 
    xmlns:content="http://purl.org/rss/1.0/modules/content/">
  <channel>
    <title>` + title + `</title>
    <description>` + description + `</description>
    <link>https://newscriber.unityflow.ai</link>
    <language>` + lang + `</language>
    <itunes:author>NewScriber AI</itunes:author>
    <itunes:summary>` + description + `</itunes:summary>
    <itunes:owner>
      <itunes:name>NewScriber AI</itunes:name>
      <itunes:email>podcast@unityflow.ai</itunes:email>
    </itunes:owner>
    <itunes:explicit>no</itunes:explicit>
    <itunes:category text="News"/>
    <itunes:image href="` + coverURL + `"/>
    <image>
      <url>` + coverURL + `</url>
      <title>` + title + `</title>
      <link>https://newscriber.unityflow.ai</link>
    </image>
    <pubDate>` + nowRFC + `</pubDate>
  </channel>
</rss>`
}

func formatSourcesList(sources []string, lang string) string {
	if len(sources) == 0 {
		switch lang {
		case "it":
			return "TechCrunch e altre fonti principali"
		case "fr":
			return "TechCrunch et d'autres sources majeures"
		case "bn":
			return "TechCrunch এবং অন্যান্য শীর্ষ সংবাদ মাধ্যম"
		default:
			return "TechCrunch and other top tech outlets"
		}
	}
	// Unique sources
	unique := make(map[string]bool)
	var list []string
	for _, s := range sources {
		if s != "" && !unique[s] {
			unique[s] = true
			list = append(list, s)
		}
	}
	if len(list) == 0 {
		switch lang {
		case "it":
			return "TechCrunch e altre fonti principali"
		case "fr":
			return "TechCrunch et d'autres sources majeures"
		case "bn":
			return "TechCrunch এবং অন্যান্য শীর্ষ সংবাদ মাধ্যম"
		default:
			return "TechCrunch and other top tech outlets"
		}
	}
	if len(list) == 1 {
		return list[0]
	}
	
	switch lang {
	case "it":
		return strings.Join(list[:len(list)-1], ", ") + " e " + list[len(list)-1]
	case "fr":
		return strings.Join(list[:len(list)-1], ", ") + " et " + list[len(list)-1]
	case "bn":
		return strings.Join(list[:len(list)-1], ", ") + " এবং " + list[len(list)-1]
	default:
		return strings.Join(list[:len(list)-1], ", ") + " and " + list[len(list)-1]
	}
}

func (h *Handler) generateEpisodeDescription(ep EpisodeRow, dialogueText string) string {
	sourcesStr := formatSourcesList(ep.SourceNames, ep.Language)
	
	// Safe truncation of dialogueText for preview
	runes := []rune(dialogueText)
	if len(runes) > 300 {
		dialogueText = string(runes[:300]) + "..."
	}

	switch ep.Language {
	case "it":
		return fmt.Sprintf("Benvenuti al bollettino quotidiano in italiano di NewScriber! Le storie di oggi sono curate da %s. Unisciti ai nostri conduttori virtuali Marco e Sofia mentre analizzano le ultime notizie: %s", sourcesStr, dialogueText)
	case "fr":
		return fmt.Sprintf("Bienvenue dans le briefing quotidien en français de NewScriber ! Les actualités d'aujourd'hui sont sélectionnées depuis %s. Rejoignez vos présentateurs virtuels Pierre et Marie pour décrypter le meilleur de la tech : %s", sourcesStr, dialogueText)
	case "bn":
		return fmt.Sprintf("নিউস্ক্রাইবার বাংলা ডেইলি ব্রিফিংয়ে আপনাদের স্বাগতম! আজকের খবরগুলো নেয়া হয়েছে %s থেকে। আমাদের এআই সঞ্চালক ফাহিম এবং নুসরাত আজকের পর্বে আলোচনা করছেন: %s", sourcesStr, dialogueText)
	default:
		return fmt.Sprintf("Welcome to today's English Daily Briefing by NewScriber! Today's stories are curated from %s. Join your virtual hosts Alex and Sam as they break down the latest updates: %s", sourcesStr, dialogueText)
	}
}

func parsePGArray(pgArr string) []string {
	if pgArr == "" || pgArr == "{}" {
		return nil
	}
	pgArr = strings.Trim(pgArr, "{}")
	if pgArr == "" {
		return nil
	}
	
	var res []string
	inQuotes := false
	var current strings.Builder
	for i := 0; i < len(pgArr); i++ {
		char := pgArr[i]
		if char == '"' {
			inQuotes = !inQuotes
			continue
		}
		if char == ',' && !inQuotes {
			res = append(res, strings.TrimSpace(current.String()))
			current.Reset()
			continue
		}
		current.WriteByte(char)
	}
	if current.Len() > 0 {
		res = append(res, strings.TrimSpace(current.String()))
	}
	return res
}
