# NewsVoice — Product Requirements Document

**Version:** 1.0 (MVP)
**Date:** 2026-05-13
**Status:** Active — Hackathon Build

---

## Problem

Newspapers that exist only in print have no audio format. Their audience cannot consume news while commuting, cooking, or otherwise occupied. They are losing reach to audio-native competitors with zero tooling to fight back.

## Solution

A fully automated pipeline that scrapes a newspaper's top articles, summarizes them, converts them to AI-generated audio, and delivers a short podcast episode to subscribers via WhatsApp and RSS — three times a day, zero human intervention required.

---

## MVP Scope

Single hardcoded newspaper. End-to-end pipeline runs automatically. 3–5 test recipients receive audio via WhatsApp. RSS feed is live and subscribable. No dashboard. No onboarding UI. Prove the pipeline works.

---

## Core Pipeline

```
Fetch Articles → Summarize → Generate Audio → Deliver
```

### Step 1: Fetch

- Source: newspaper website via Firecrawl (investigate free tier)
- Fallback: RSS feed parsing if available
- Selects top 3–5 articles per episode (by position/recency)
- Runs on cron schedule

### Step 2: Summarize

- LLM condenses each article to ~220 words
- Prompt style: broadcast radio news script, neutral tone
- LLM provider: TBD (Claude Haiku or OpenAI GPT-4o-mini shortlisted)
- Output: clean spoken prose, no bullet points, no markdown

### Step 3: Generate Audio

- Input: combined script of all summarized articles with intro/outro
- Target duration: **2–5 minutes** per episode
- TTS provider: TBD (OpenAI TTS `tts-1` shortlisted)
- Voice: single consistent AI voice, English, neutral news anchor tone
- Output format: MP3

### Step 4: Deliver

- **WhatsApp:** send MP3 file to subscriber list via WhatsApp Business API
  - Vendor: TBD (Twilio WhatsApp sandbox for hackathon)
- **RSS Feed:** upload MP3 to Cloudflare R2, update XML feed
  - Feed URL: one per newspaper tenant (future multi-tenant)
  - Compatible with Apple Podcasts, Spotify, Pocket Casts

---

## Schedule

| Episode          | Time          |
| ---------------- | ------------- |
| Morning Briefing | Early morning |
| Midday Briefing  | Lunch time    |
| Evening Briefing | Dinner time   |

- Cron-triggered, fully automated
- No human approval required before send
- Fixed schedule configured at setup time

---

## Tech Stack

| Layer             | Choice                           |
| ----------------- | -------------------------------- |
| Language          | Go                               |
| Article fetching  | Firecrawl REST API (investigate) |
| Summarization LLM | TBD (Claude Haiku / GPT-4o-mini) |
| Text-to-Speech    | TBD (OpenAI TTS shortlisted)     |
| Audio hosting     | Cloudflare R2                    |
| WhatsApp delivery | TBD (Twilio sandbox shortlisted) |
| RSS               | Hand-generated XML, hosted on R2 |
| Scheduler         | System cron                      |

---

## Business Model

- **Customer:** Newspaper / media publication
- **Value proposition:** Distribution — get audio content to their existing audience without hiring audio staff
- **Pricing (future):** Flat monthly SaaS fee per publication, usage-capped on episodes/month
- **MVP:** Single newspaper, no billing

---

## Out of Scope (MVP)

- Multi-tenant onboarding dashboard
- Editor approval UI
- Subscriber opt-in flow / consent management
- Analytics / listen tracking
- Voice cloning or custom branded voice
- Non-English language support
- Breaking news triggers (vs. fixed schedule)
- Mobile app

---

## Success Criteria (Hackathon Demo)

- [ ] Pipeline runs end-to-end without manual intervention
- [ ] Episode is 2–5 minutes long
- [ ] Audio sounds broadcast-quality (no robotic artifacts)
- [ ] WhatsApp delivery reaches test recipients successfully
- [ ] RSS feed validates and loads in a podcast app
- [ ] 3 scheduled episodes fire correctly in a 24-hour window

---

## Open Decisions

| Decision        | Status | Notes                                     |
| --------------- | ------ | ----------------------------------------- |
| LLM provider    | TBD    | Claude Haiku vs GPT-4o-mini               |
| TTS provider    | TBD    | OpenAI TTS vs ElevenLabs                  |
| WhatsApp vendor | TBD    | Investigate Twilio sandbox vs Meta direct |
| Scraping method | TBD    | Firecrawl vs RSS feed parsing             |

---

## V2 Roadmap (Post-Hackathon)

1. Multi-tenant newspaper onboarding with dashboard
2. Subscriber opt-in flow (WhatsApp consent management)
3. Editor approval UI before send
4. Per-newspaper branded voice
5. Bengali / multilingual TTS support
6. Listen analytics and delivery receipts
7. Breaking news trigger (threshold-based episode generation)
8. Pricing and billing infrastructure
