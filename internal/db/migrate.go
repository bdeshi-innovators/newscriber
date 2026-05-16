package db

import (
	"context"
	"database/sql"
	"fmt"
)

const schema = `
CREATE TABLE IF NOT EXISTS users (
    phone_number  VARCHAR(32) PRIMARY KEY,
    language_pref VARCHAR(2)  NOT NULL DEFAULT 'en'
                  CHECK (language_pref IN ('en','it','bn')),
    timezone      TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Parsed-article checkpoint. The n8n scrape workflow writes rows here after
-- Firecrawl extract; the TTS/translate/broadcast sub-workflow consumes them
-- and stamps the per-stage *_at columns. If TTS or send fails, the row
-- remains the resume point — re-running the downstream workflow picks up
-- where it left off without re-paying Firecrawl.
CREATE TABLE IF NOT EXISTS news_items (
    id              BIGSERIAL PRIMARY KEY,
    fingerprint     TEXT UNIQUE NOT NULL,            -- sha256(article_url)[:16]
    source_name     TEXT        NOT NULL,
    article_url     TEXT        NOT NULL,
    language        VARCHAR(2)  NOT NULL,            -- source language; matches users.language_pref
    headline        TEXT        NOT NULL,
    dek             TEXT,
    body            TEXT        NOT NULL,            -- raw prose from Firecrawl extract
    score           INTEGER,                         -- relevance score (1-10)
    llm_summary     TEXT,                            -- polished broadcast prose from LLM
    summarized_at   TIMESTAMPTZ,                     -- set when LLM summary completes
    published_date  DATE,
    topics          TEXT[]      NOT NULL DEFAULT '{}',
    scraped_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    skipped_at      TIMESTAMPTZ,                     -- set if article was rejected by agent
    translated_at   TIMESTAMPTZ,                     -- set when downstream finishes translating
    tts_at          TIMESTAMPTZ,                     -- set when MP3 is uploaded
    mp3_url         TEXT,                            -- public URL for WhatsApp media
    broadcast_at    TIMESTAMPTZ                      -- set when /broadcast fans out
);
CREATE INDEX IF NOT EXISTS news_items_unsent
    ON news_items (scraped_at)
    WHERE broadcast_at IS NULL;
CREATE INDEX IF NOT EXISTS news_items_unsummarized
    ON news_items (scraped_at)
    WHERE llm_summary IS NULL;

CREATE TABLE IF NOT EXISTS episodes (
    id           BIGSERIAL    PRIMARY KEY,
    language     VARCHAR(2)   NOT NULL,
    script       TEXT         NOT NULL,   -- full episode prose, ready for TTS
    source_names TEXT[]       NOT NULL DEFAULT '{}',
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    tts_at       TIMESTAMPTZ,             -- set when MP3 is generated (Step 3)
    mp3_url      TEXT                     -- set when MP3 is uploaded (Step 3)
);
`

func Migrate(ctx context.Context, conn *sql.DB) error {
	if _, err := conn.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("apply schema: %w", err)
	}
	return nil
}
