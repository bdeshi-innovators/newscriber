package webhook

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"
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
}

func (h *Handler) UpdateRSSFeed(ctx context.Context, lang string) error {
	slog.Info("updating RSS feed", "lang", lang)

	// 1. Fetch latest episodes for the language (limit to latest 50 to keep feed size lean)
	var rows *sql.Rows
	var err error
	if lang == "global" {
		rows, err = h.db.QueryContext(ctx, 
			"SELECT id, script, mp3_url, created_at, COALESCE(title, '') as title, COALESCE(description, '') as description, episode_number, language FROM episodes WHERE mp3_url IS NOT NULL AND status = 'published' ORDER BY created_at DESC LIMIT 100;",
		)
	} else {
		rows, err = h.db.QueryContext(ctx, 
			"SELECT id, script, mp3_url, created_at, COALESCE(title, '') as title, COALESCE(description, '') as description, episode_number, language FROM episodes WHERE language = $1 AND mp3_url IS NOT NULL AND status = 'published' ORDER BY created_at DESC LIMIT 50;", 
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
		if err := rows.Scan(&ep.ID, &ep.Script, &ep.MP3URL, &ep.CreatedAt, &ep.Title, &ep.Description, &ep.EpisodeNumber, &ep.Language); err != nil {
			return fmt.Errorf("scan episode: %w", err)
		}
		episodes = append(episodes, ep)
	}

	// 2. Generate XML string
	xmlString := h.buildRSSXML(lang, episodes)

	// 3. Upload XML to Cloudflare R2
	filename := fmt.Sprintf("feed_%s.xml", lang)
	publicURL, err := h.tts.UploadXML(ctx, filename, []byte(xmlString))
	if err != nil {
		return fmt.Errorf("upload rss xml: %w", err)
	}

	slog.Info("RSS feed updated successfully", "lang", lang, "url", publicURL)
	return nil
}

func (h *Handler) buildRSSXML(lang string, episodes []EpisodeRow) string {
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
    <itunes:image href="https://images.unsplash.com/photo-1590602847861-f357a9332bbc?w=1400"/>
    <image>
      <url>https://images.unsplash.com/photo-1590602847861-f357a9332bbc?w=1400</url>
      <title>` + title + `</title>
      <link>https://newscriber.unityflow.ai</link>
    </image>
    <pubDate>` + nowRFC + `</pubDate>
`

	for _, ep := range episodes {
		// Clean description: limit prose snippet for podcast directories
		snippet := ep.Description
		if snippet == "" {
			snippet = ep.Script
		}
		if len(snippet) > 400 {
			snippet = snippet[:400] + "..."
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
      <title>` + epTitle + `</title>
      <description><![CDATA[` + snippet + `]]></description>
      <pubDate>` + pubDate + `</pubDate>
      <enclosure url="` + ep.MP3URL + `" length="10000000" type="audio/mpeg"/>
      <guid isPermaLink="false">` + ep.MP3URL + `</guid>
      <itunes:explicit>no</itunes:explicit>
    </item>
`
	}

	xml += `  </channel>
</rss>`

	return xml
}
