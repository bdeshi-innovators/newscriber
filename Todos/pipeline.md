# VoiceScribe — Pipeline Specification

## Overview

Two parallel runs per day — one per language — each triggered by a cron at the
user's local 7am:

| Run     | Cron (UTC) | Language | Audience                  |
| ------- | ---------- | -------- | ------------------------- |
| Italian | 06:00      | `it`     | `+39` users → Europe/Rome |
| English | 07:00      | `en`     | `+44` and others          |

Both runs execute the same workflow, parameterised by language tag.

---

## Stages

### 1. Cron Trigger

- Two n8n cron nodes, each tagged with a target language (`it` / `en`)
- Italian fires at 06:00 UTC → lands 7am Rome time (±1hr DST)
- English fires at 07:00 UTC

---

### 2. Fetch — Firecrawl

- Scrape index pages + full article bodies from configured sources
- Sources: TechCrunch, VentureBeat, BBC Tech, MIT Tech Review, Al Jazeera
- Output: list of `{ url, title, body, source }` objects

---

### 3. Deduplicate — Postgres

- SHA-256 fingerprint each article body
- Check against `news_items` table
- Drop already-seen articles; insert new rows with `fetched_at` timestamp
- Output: list of net-new articles only

---

### 4. AI Agent — Azure OpenAI GPT-4o

The autonomous reasoning layer. Three steps in sequence:

**Step 1 — Score for relevance**

- LLM rates each article 1–10 for relevance to tech entrepreneurs and AI leaders
- Factors: novelty, business impact, relevance to tech industry, balancing past events and future expectations.

**Step 2 — Rank and select**

- Keep top 7–10 stories
- Drop thematic duplicates
- Ensure a balanced mix across topics

**Step 3 — Broadcast decision**

- Is there enough novel, high-scoring content to justify an episode?
- **NO** → write `skipped_at` to DB, exit workflow silently. No message sent to subscribers.
- **YES** → continue to next stage

---

### 5. Summarise — Azure OpenAI

- Each selected article → ~70-word spoken-language summary
- Prompt style: short sentences, active voice, no jargon, audio-ready
- Write `summarized_at` to `news_items` row

---

### 6. Translate — Azure OpenAI

- **English run**: no translation, pass through
- **Italian run**: translate each summary `en → it`
- Write `translated_at` to `news_items` row

---

### 7. Episode Assembly — Azure OpenAI

Stitch summaries into a single spoken script:

```
Intro (15s) → Story 1 → Story 2 → Story 3 → [Story 4] → [Story 5] → Outro (10s)
```

- Total target length: ~2–3 minutes of audio
- Intro/outro are language-specific (pre-written templates, filled by LLM)
- Output: one complete plain-text script per language

---

### 8. TTS — ElevenLabs or Azure Speech

- Script → MP3
- Separate voice persona per language (consistent across episodes)
  - English: one voice
  - Italian: one voice
- Write `tts_at` to episode row

---

### 9. Upload — Cloudflare R2

- Upload MP3 to R2 bucket under path `episodes/{date}-{lang}.mp3`
- Capture public HTTPS URL
- Write URL to `episodes.mp3_url` in Postgres

---

### 10. Update RSS Feed — n8n HTTP node

- Fetch current `feed-en.xml` or `feed-it.xml` from R2
- Prepend new `<item>` block:
  ```xml
  <item>
    <title>VoiceScribe — {date} {lang}</title>
    <pubDate>{RFC822 date}</pubDate>
    <enclosure url="{mp3_url}" type="audio/mpeg"/>
    <guid>{mp3_url}</guid>
  </item>
  ```
- Re-upload updated XML to R2
- RSS feed is valid for any podcast app (Apple Podcasts, Spotify, Pocket Casts)

---

### 11. WhatsApp Push — Twilio

- Query Postgres: `SELECT phone_number FROM users WHERE language_pref = ?`
- Send one text message per subscriber:
  - English: `"Your morning briefing is ready 🎙️ Listen here: {feed_url}"`
  - Italian: `"Il tuo briefing mattutino è pronto 🎙️ Ascolta qui: {feed_url}"`
- Log each send in `deliveries(episode_id, phone_number, sent_at, status)`
- Write `broadcast_at` on the episode row

---

## User Onboarding (inbound — already built)

Triggered when a user texts the WhatsApp number for the first time:

1. Any message → language menu: `1 = English · 2 = Italian`
2. User replies `1` or `2` → stored in `users(phone_number, language_pref, timezone)`
3. Timezone inferred from phone prefix: `+39` → `Europe/Rome`, `+44` → `Europe/London`
4. Confirmation message sent with podcast feed link

---

## Database checkpoints

| Table        | Column              | Written at stage |
| ------------ | ------------------- | ---------------- |
| `news_items` | `fetched_at`        | 3 — Deduplicate  |
| `news_items` | `summarized_at`     | 5 — Summarise    |
| `news_items` | `translated_at`     | 6 — Translate    |
| `episodes`   | `tts_at`            | 8 — TTS          |
| `episodes`   | `mp3_url`           | 9 — Upload       |
| `episodes`   | `broadcast_at`      | 11 — Broadcast   |
| `deliveries` | `sent_at`, `status` | 11 — Broadcast   |

If the pipeline fails at any stage, the checkpoint columns show exactly where it
stopped — the next run can resume from that point rather than starting over.

---

## Build order (remaining work)

| Priority | What                                 | Where                 |
| -------- | ------------------------------------ | --------------------- |
| 1        | Agent scoring + decision node        | n8n                   |
| 2        | Episode assembly prompt              | n8n                   |
| 3        | TTS node (ElevenLabs REST call)      | n8n                   |
| 4        | R2 upload node                       | n8n                   |
| 5        | RSS XML update node                  | n8n                   |
| 6        | WhatsApp push text (not MP3)         | n8n + Go `/broadcast` |
| 7        | Duplicate cron + language tag for IT | n8n                   |
| 8        | Timezone inference on user signup    | Go webhook            |
