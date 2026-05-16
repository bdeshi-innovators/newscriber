# NewsVoice — MVP Checklist (PRD vs Repo Status)

> Last updated: 2026-05-14
> Legend: ✅ Done · 🔶 Partial / scaffolded · ❌ Missing

---

## Pipeline Step 1 — Fetch Articles

| Task | Status | Notes |
|------|--------|-------|
| Firecrawl integration for scraping newspaper index page | ✅ Done | `n8n/workflows/news-pipeline-v2.json` — `Firecrawl: scrape index` node |
| Firecrawl per-article extraction (structured JSON output) | ✅ Done | `Firecrawl: extract article` node with TTS-shaped prompt |
| Source list (hardcoded for MVP) | ✅ Done | BBC News + Al Jazeera in the Code node |
| Filter top 3–5 articles per run | ✅ Done | Caps at **5** per source in V2 |
| Cron schedule (3× daily) | ✅ Done | V2 has 3 specific triggers: 07:00, 13:00, 19:00 UTC |
| RSS fallback if Firecrawl unavailable | ❌ Missing | Not implemented |
| Dedupe insert to `news_items` Postgres table | ✅ Done | Node is **enabled** in V2 |

---

## Pipeline Step 2 — Summarize

| Task | Status | Notes |
|------|--------|-------|
| LLM summarization to ~220 words | ✅ Done | Azure OpenAI node in V2 produces polished ~220-word prose |
| Broadcast radio script style (neutral tone, no bullets/markdown) | ✅ Done | Firecrawl + Azure OpenAI prompts enforce plain spoken prose |
| LLM provider decision (Claude Haiku / GPT-4o-mini) | ✅ Done | **Azure OpenAI / Kimi-K2.5** selected for hackathon |
| Intro/outro script wrapping all articles into one episode | ✅ Done | `Azure OpenAI: Assemble Episode` node in V2 |

---

## Pipeline Step 3 — Generate Audio

| Task | Status | Notes |
|------|--------|-------|
| TTS call (OpenAI TTS `tts-1` or ElevenLabs) | ❌ Missing | No TTS node or Go code exists anywhere |
| MP3 output generation | ❌ Missing | `mp3_url` column exists in schema but nothing writes to it |
| 2–5 minute episode target duration | ❌ Missing | No audio assembly/trimming logic |
| Single consistent AI voice, neutral news anchor tone | ❌ Missing | No voice config |
| Upload MP3 to Cloudflare R2 | ❌ Missing | No R2 client or upload workflow |

---

## Pipeline Step 4 — Deliver

| Task | Status | Notes |
|------|--------|-------|
| WhatsApp webhook receiver (inbound messages) | ✅ Done | `internal/webhook/handler.go` + parser for Twilio & Meta formats |
| Subscriber list stored in Postgres | ✅ Done | `users` table with phone + language pref |
| WhatsApp opt-in flow (language selection menu) | ✅ Done | 1/2/3 menu → UpsertUser in handler |
| Outbound WhatsApp MP3 send to all subscribers | ❌ Missing | Handler returns mock URL `voicescribe.example/…`; no real Twilio/Meta outbound call |
| Fan-out broadcast endpoint (`POST /broadcast`) | ❌ Missing | No `/broadcast` route exists in `cmd/webhook/main.go` |
| RSS feed XML generation | ❌ Missing | No RSS builder; schema notes it as future but nothing is implemented |
| RSS feed hosted on Cloudflare R2 | ❌ Missing | No R2 integration at all |
| RSS compatible with Apple Podcasts / Spotify / Pocket Casts | ❌ Missing | Depends on RSS above |

---

## Scheduling & Automation

| Task | Status | Notes |
|------|--------|-------|
| 3 fixed daily episodes (morning / midday / evening) | ❌ Missing | n8n fires every 2h, not at specific times |
| Fully automated, zero human intervention | 🔶 Partial | n8n cron exists; downstream TTS/send steps are manual gaps |
| System cron as scheduler (per tech stack) | 🔶 Partial | n8n schedule trigger is used instead of system cron |

---

## Infrastructure

| Task | Status | Notes |
|------|--------|-------|
| Go HTTP server (webhook receiver) | ✅ Done | `cmd/webhook/main.go` |
| PostgreSQL schema with migrations | ✅ Done | `internal/db/migrate.go` — `users` + `news_items` tables |
| Docker Compose stack (app + db + n8n) | ✅ Done | `docker-compose.yml` |
| Dockerfile | ✅ Done | `Dockerfile` |
| `.env` / `.env.example` configuration | ✅ Done | All required vars documented |
| Cloudflare R2 bucket + public URL | ❌ Missing | No R2 credentials or client wired up |
| **HTTPS / TLS** | ❌ Missing | Currently serving over HTTP on IP; needs domain + Caddy SSL |
| **Internal service isolation** | ✅ Done | Services restricted to `voicescribe-net` docker network |
| **Host port hardening** | ❌ Missing | DB and n8n ports still bound to `0.0.0.0` in `docker-compose.yml` |
| **Webhook Signature Validation** | ❌ Missing | No logic to verify Twilio/Meta signatures yet |

---

## Hackathon Demo Success Criteria (from PRD §Success Criteria)

| Criterion | Status |
|-----------|--------|
| Pipeline runs end-to-end without manual intervention | ❌ Not yet — TTS, upload, broadcast steps missing |
| Episode is 2–5 minutes long | ❌ No audio generated yet |
| Audio sounds broadcast-quality (no robotic artifacts) | ❌ No audio yet |
| WhatsApp delivery reaches test recipients successfully | ❌ Outbound send not implemented |
| RSS feed validates and loads in a podcast app | ❌ RSS not implemented |
| 3 scheduled episodes fire correctly in a 24-hour window | ❌ Schedule not aligned to 3× daily |

---

## Open Decisions (from PRD §Open Decisions)

| Decision | Status |
|----------|--------|
| LLM provider (Claude Haiku vs GPT-4o-mini) | ❌ Still TBD |
| TTS provider (OpenAI TTS vs ElevenLabs) | ❌ Still TBD |
| WhatsApp vendor (Twilio vs Meta direct) | 🔶 Code supports both parsers; no outbound wired |
| Scraping method (Firecrawl vs RSS) | ✅ Decided — Firecrawl chosen |

---

## Summary — What's Left to Build

### 🔴 Critical / Blocking (pipeline cannot complete without these)

- [ ] **TTS workflow** — n8n sub-workflow calling OpenAI TTS or ElevenLabs per `news_items` row
- [ ] **MP3 upload to Cloudflare R2** — store public URL back to `news_items.mp3_url`
- [ ] **`/broadcast` endpoint** — reads unbroadcast rows, fans out WhatsApp sends via Twilio outbound API
- [ ] **Outbound WhatsApp send** — real Twilio `Messages.create()` call with `mediaUrl`
- [ ] **Episode assembly** — combine multiple article bodies into one intro+articles+outro script before TTS

### 🟡 Important (PRD-required but hackathon may workaround)

- [ ] **Fix cron schedule** — change from every-2h to 3 specific daily slots (morning/midday/evening)
- [ ] **Article count cap** — reduce from 10 to 3–5 per run
- [ ] **Enable Postgres dedupe node** in n8n workflow
- [ ] **RSS feed builder** — generate and upload podcast XML to R2

### 🟢 Nice-to-have / Polish

- [ ] Replace hardcoded source list Code node with Google Sheets node
- [ ] RSS fallback scraping if Firecrawl fails
- [ ] `news_items` `translated_at` / `tts_at` / `broadcast_at` lifecycle stamping (schema ready, no writer yet)

### 🔒 Security & Deployment Hardening

- [ ] **Attach Domain Name** — Redirect traffic to `voicescribe.yourdomain.com`.
- [ ] **Enable HTTPS** — Update Caddyfile to use the domain and trigger Let's Encrypt.
- [ ] **Restrict Docker Ports** — Change `ports:` to `127.0.0.1:15678:5678` for n8n/DB.
- [ ] **Implement Webhook Verification** — Reject unauthenticated requests from Twilio/Meta.
