# VoiceScribe — News Pipeline V2 (n8n + Firecrawl + Azure OpenAI)

V2 of the news-ingest pipeline. It handles fetching, summarization via LLM, and episode assembly.

## The pipeline

```
[Schedule 07:00, 13:00, 19:00 UTC]
        │
        ▼
[Source list]  ← hardcoded (BBC, Al Jazeera)
        │
        ▼
[Firecrawl /scrape index]  → discovers article URLs
        │
        ▼
[Filter article URLs]  regex per source, cap at 5
        │
        ▼
[Firecrawl /scrape article]  structured extract (180-220 words)
        │
        ▼
[Azure OpenAI Summarize]  rewrites to polished broadcast prose (~220 words)
        │
        ▼
[Postgres save news_item]  dedupe via fingerprint
        │
        ▼
[Aggregate summaries]  collects all articles from run
        │
        ▼
[Azure OpenAI Assemble]  Intro + Articles + Outro (600-900 words)
        │
        ▼
[Postgres save episode]  Ready for Step 3 (TTS)
```

## Credentials to set up in n8n

### 1. Firecrawl Bearer (required)
- **Credential type:** HTTP Header Auth
- **Name:** `Firecrawl Bearer`
- **Header name:** `Authorization`
- **Header value:** `Bearer fc-...`

### 2. Azure OpenAI Key (required)
- **Credential type:** HTTP Header Auth
- **Name:** `Azure OpenAI Key`
- **Header name:** `api-key`
- **Header value:** (use your Azure OpenAI API key)

### 3. VoiceScribe Postgres (required)
- **Credential type:** Postgres
- **Host:** `db`
- **Port:** `5432`
- **Database:** `voicescribe`
- **User / Password:** match `.env`

## What's next
- **TTS** per episode → MP3 (Step 3)
- **Upload** to R2 (Step 3)
- **Broadcast** to WhatsApp (Step 4)

Each of those is a separate sub-workflow that consumes rows where
`broadcast_at IS NULL` from `news_items`.
