# NewScriber (formerly VoiceScribe) — Multilingual Automated News Briefing & Podcast Network

NewScriber is a high-fidelity, autonomous, end-to-end multilingual AI news editor, podcast scriptwriter, and voice briefing delivery network. 

The system automates the entire ingestion-to-ear pipeline: it extracts tech and business news from multiple websites using **Firecrawl**, selects and ranks the best stories using an **Azure OpenAI-powered Editor Agent**, summarizes them, compiles them into natural dual-host conversational podcast scripts, renders lifelike synthetic voice dialogues using **Gemini TTS via OpenRouter**, transcodes the audio into multiple high-quality formats (WAV, MP3, OGG) using **ffmpeg**, hosts them on **Minio/Cloudflare R2**, generates self-healing **iTunes-compliant RSS feeds**, serves a beautiful web **Visualizer Dashboard**, and delivers briefings directly to subscribed users via **WhatsApp webhooks**.

---

## Core Stack & Technologies

NewScriber is fully containerized, secure, and optimized for both local development and remote cloud deployments.

### 1. Go Webhook & Orchestration Engine (`webhook-app`)
*   **Performance & Concatenation**: Acts as the central high-performance server. It processes incoming WhatsApp webhook payloads (supporting both Twilio and Meta Cloud API formats), handles user registrations in PostgreSQL, orchestrates TTS audio renders, transcodes raw PCM data, and serves the Web Visualizer.
*   **ffmpeg Transcoding**: Integrates with local `ffmpeg` to stitch individual voice dialogues and transcode raw 16-bit LE mono PCM @ 24kHz streams into:
    *   `WAV` (PCM raw format)
    *   `MP3` (`audio/mpeg` at high bitrate)
    *   `OGG` (`audio/ogg` encoded with Opus)
*   **RSS Generator**: Dynamically compiles and uploads iTunes-compliant XML podcast feeds (`feed_en.xml`, `feed_it.xml`, etc.) to S3 with automatic self-healing routines to bypass XML formatting corruptions.

### 2. n8n Workflow Orchestrator
*   **News Pipeline V4**: Serves as the cron-driven and manual visual workflow editor. It manages index crawling, article parsing, LLM editing and summarizing, multi-language dialogue script synthesis, dialogue review, Go `/tts` invocations, and database persistence.
*   **Automated Bootstrap**: Pre-loads workflows and connection credentials using `scripts/n8n-init.sh` upon stack boot, ensuring a zero-manual-configuration developer experience.

### 3. Database Layer (PostgreSQL)
*   `users`: Persists user phone numbers, language preferences, timezones, and registration timestamps.
*   `news_items`: Acts as an article cache and state machine. Stores scraped metadata, raw bodies, and AI summaries. Unsummarized and unsent article indices ensure high resiliency—if downstream steps fail, the pipeline resumes without re-scraping.
*   `episodes`: Stores assembled podcast transcripts, S3 audio URLs, title, description, episode number, and publication statuses.

### 4. Advanced AI & Media Integration
*   **Firecrawl v2**: Scrapes publication index feeds and extracts raw markdown articles, stripping paywalls, listicles, and clutter.
*   **Azure OpenAI (Kimi-K2.5)**: Powers the **Autonomous AI Editor** to filter, select, and summarize the best 7-10 stories from the past 24 hours, and compiles them into alternating dual-host scripts.
*   **OpenRouter (Gemini-3.1-Flash-TTS)**: Synthesizes high-fidelity, expressive natural speech using gender-balanced, localized voice mappings.
*   **Minio / Cloudflare R2**: Securely hosts public WAV/MP3/OGG podcast files, custom cover arts, and RSS XML feeds.

---

## System Architecture

NewScriber connects two decoupled, highly optimized systems: the **WhatsApp Onboarding Webhook** and the **Autonomous News & Podcast Pipeline**.

```
                           [ WhatsApp User ]
                             ▲           │
                  Outbound   │           │   Inbound Text
                  Audio/Text │           ▼   (Twilio or Meta JSON)
                       ┌─────┴───────────┴─────────────┐
                       │      Twilio / Meta APIs       │
                       └─────▲───────────┬─────────────┘
                             │           │ HTTPS POST (/webhook/whatsapp)
                             │           ▼
  ┌──────────────────────────┼────────────────────────────────────────────────────────┐
  │ Go webhook-app           │                                                        │
  │                          │                                                        │
  │  ┌───────────────────────┴─┐   ┌──────────────────────────┐   ┌─────────────────┐ │
  │  │  /webhook/whatsapp      │   │  /visualizer             │   │  /rss/generate  │ │
  │  │                         │   │  (Control Dashboard)     │   │                 │ │
  │  │  • Receives texts       │   │  • Plays WAV/MP3/OGG     │   │  • Builds XML   │ │
  │  │  • Updates preferences  │   │  • Shows transcript      │   │  • Self-heals   │ │
  │  │  • Replies latest MP3   │   │  • Triggers new drops    │   │  • Uploads S3   │ │
  │  └─────────────────────────┘   └──────────────────────────┘   └─────────┬───────┘ │
  │                                                                         │         │
  │  ┌─────────────────────────┐   ┌──────────────────────────┐             │         │
  │  │  /tts                   │◄──┤  /publish                │             │         │
  │  │                         │   │                          │             │         │
  │  │  • Maps Gemini Voices   │   │  • Saves drafts          │             │         │
  │  │  • Fetches OpenRouter   │   │  • Sets Titles/Descs     │             │         │
  │  │  • Concatenates PCM     │   │  • Triggers RSS update   │             │         │
  │  │  • ffmpeg (MP3/OGG/WAV) │   └──────────────────────────┘             │         │
  │  └──────────┬──────────────┘                                            │         │
  └─────────────┼───────────────────────────────────────────────────────────┼─────────┘
                │ PutObject                                                 │ PutObject
                ▼                                                           ▼
  ┌───────────────────────────────────────────────────────────────────────────────────┐
  │ S3-Compatible Object Storage (Minio Local / Cloudflare R2 Cloud)                   │
  │  • podcast-covers/   • episode_en_*.mp3/ogg/wav   • feed_en.xml / feed_global.xml │
  └───────────────────────────────────────────────────────────────────────────────────┘
                                    ▲
                                    │ SQL (Query raw / Save Episodes)
                                    ▼
  ┌───────────────────────────────────────────────────────────────────────────────────┐
  │ PostgreSQL (Port: 55432)                                                          │
  │  • users               • news_items (fingerprint cache)   • episodes (briefs)     │
  └─────────────────────────────────▲─────────────────────────────────────────────────┘
                                    │
                                    │ SQL inserts & selects
                                    ▼
  ┌───────────────────────────────────────────────────────────────────────────────────┐
  │ n8n Workflow Engine (cron trig or manual /api/trigger proxy)                      │
  │                                                                                   │
  │   1. Source List ────► 2. Firecrawl Index Scrape ────► 3. Filter Uncached URLs   │
  │                                                                 │                 │
  │   5. Azure OpenAI Select & Rank ◄──── 4. Firecrawl Extract ◄────┘                 │
  │       (7-10 best tech stories)                                                    │
  │               │                                                                   │
  │               ▼                                                                   │
  │   6. LLM Summaries (Cached check) ──► 7. Localized Summaries & Podcast Descriptions│
  │                                                                                   │
  │   9. Azure OpenAI Dialogue Assembly ◄─── 8. Prep Multi-Lang Dialogue Prompt       │
  │      • Alex & Sam (EN)      • Sofia & Marco (IT)                                  │
  │      • Marie & Pierre (FR)  • Nusrat & Fahim (BN)                                 │
  │               │                                                                   │
  │               ▼                                                                   │
  │   10. Dialogue Flow Review & Schema Guard                                         │
  │               │                                                                   │
  │               ▼                                                                   │
  │   11. HTTP Post (/tts webhook-app) ──► 12. Save Episode ──► 13. Regenerate RSS    │
  └───────────────────────────────────────────────────────────────────────────────────┘
```

---

## Podcast Host Personas & Voices

NewScriber features gender-balanced dual-host alternating dialogues tailored to each target language.

| Language | Code | Host 1 (Female) | Host 2 (Male) | Gemini Voices | Description |
| :--- | :--- | :--- | :--- | :--- | :--- |
| **English** | `en` | Alex (Warm & analytical) | Sam (Enthusiastic & tech-savvy) | `Zephyr` (Alex), `Puck` (Sam) | English Tech Briefing |
| **Italiano** | `it` | Sofia (Warm & analytical) | Marco (Enthusiastic & tech-savvy) | `Kore` (Sofia), `Umbriel` (Marco) | Bollettino Italiano |
| **Français** | `fr` | Marie (Warm & analytical) | Pierre (Enthusiastic & tech-savvy) | `Leda` (Marie), `Orus` (Pierre) | Briefing Français |
| **বাংলা** | `bn` | Nusrat (Warm & analytical) | Fahim (Enthusiastic & tech-savvy) | `Aoede` (Nusrat), `Charon` (Fahim) | বাংলা দৈনিক ব্রিফিং |

---

## Local Setup & Installation

### 1. Prerequisites
Ensure you have the following installed on your machine:
*   [Docker & Docker Compose](https://docs.docker.com/engine/install/)
*   An [OpenRouter API Key](https://openrouter.ai/) (required for Gemini TTS rendering)
*   An [Azure OpenAI Service Endpoint](https://azure.microsoft.com/en-us/products/ai-services/openai-service) (or an equivalent OpenAI-compliant model endpoint)

### 2. Clone and Setup Environment Variables
Clone the repository and copy the environment template:
```sh
cp .env.example .env
```
Open `.env` and fill in the required keys:
```env
# Database Credentials
POSTGRES_USER=voicescribe
POSTGRES_PASSWORD=voicescribe_secure_password
POSTGRES_DB=voicescribe

# Host Port Assignments
DB_HOST_PORT=55432
WEBHOOK_HOST_PORT=18080
N8N_HOST_PORT=15678

# OpenRouter (For Gemini TTS Voice Generation)
OPENROUTER_API_KEY=your_openrouter_api_key_here

# Firecrawl API Key
FIRECRAWL_API_KEY=your_firecrawl_api_key_here

# Azure OpenAI Credentials (For Article Selection, Summaries & Script Writing)
AZURE_OPENAI_ENDPOINT=https://<your-resource-name>.openai.azure.com
AZURE_OPENAI_DEPLOYMENT=Kimi-K2.5
AZURE_OPENAI_API_VERSION=2024-12-01-preview
AZURE_OPENAI_API_KEY=your_azure_openai_key_here

# Twilio & Meta WhatsApp Credentials (Optional)
TWILIO_ACCOUNT_SID=ACXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX
TWILIO_AUTH_TOKEN=your_twilio_auth_token
TWILIO_WHATSAPP_NUMBER=whatsapp:+14155238886
META_WHATSAPP_NUMBER=+14155238886
```

### 3. Build & Launch the Container Stack
Start the namespaced containers:
```sh
docker compose up --build -d
```
This boots up four services in the isolated `voicescribe-net` network:
*   `voicescribe-db` (Postgres 15, exposing `55432` to the host).
*   `voicescribe-webhook` (Go Webhook App, exposing `18080` to the host).
*   `voicescribe-n8n` (n8n Workflow Engine, exposing `15678` to the host).
*   `minio` (S3 object storage, if configured locally).

To check the service health:
```sh
curl http://localhost:18080/healthz
# Output: ok
```

---

## API Endpoints Reference

The Go `webhook-app` exposes a variety of HTTP endpoints to control and visualize the podcast network.

### 1. `GET /visualizer`
*   **Description**: Serves a premium, responsive web visualizer dashboard (`visualizer.html`).
*   **Features**: Displays a full list of all created podcast drafts and published episodes, dynamic source attributions, interactive HTML5 audio players for MP3, OGG, and WAV streams, and alternating dialogue transcripts. Includes a "Trigger New Episode" button to launch the pipeline.

### 2. `POST /webhook/whatsapp`
*   **Description**: Relay endpoint for inbound WhatsApp messages.
*   **Formats**: Accepts Twilio form-encoded or Meta Cloud API JSON structures.
*   **Logic**:
    *   Saves/updates the user's phone number and language selection in the database.
    *   Triggers Twilio TwiML or Meta reply confirmations returning the latest podcast episode audio.

### 3. `POST /tts`
*   **Description**: Generates speech audio files from a raw dialogues script.
*   **Payload**:
    ```json
    {
      "script": [
        {"speaker": "Alex", "text": "Hello and welcome to NewScriber!"},
        {"speaker": "Sam", "text": "Hey Alex! Glad to be here."}
      ],
      "language": "en",
      "filename": "episode_en_20260519.wav"
    }
    ```
*   **Logic**: 
    1. Maps each `speaker` to a native Gemini TTS voice profile.
    2. Sequentially queries OpenRouter's Speech API.
    3. Concatenates the PCM chunks into one master stream.
    4. Invokes `ffmpeg` to encode the PCM into WAV, MP3, and OGG.
    5. Uploads all three streams to the S3 bucket and returns their public URLs.

### 4. `GET /rss/generate`
*   **Description**: Compiles and uploads podcast XML feeds.
*   **Logic**: Iterates through all published database episodes for `en`, `it`, `fr`, `bn`, and a combined `global` stream. Creates an iTunes-compliant XML structure, executes syntax self-healing, and saves it to S3 as `feed_en.xml`, `feed_global.xml`, etc.

### 5. `GET /episodes` or `/api/episodes`
*   **Description**: Serves a JSON API endpoint returning all episodes in the database.
*   **Logic**: Dynamically aggregates and maps preceding 2-hour scraped news articles inside the JSON structure.

### 6. `POST /trigger` or `/api/trigger`
*   **Description**: Proxy endpoint that triggers the n8n News Pipeline workflow.
*   **Logic**: Calls `http://n8n:5678/webhook/VoiceScribeV4/webhook/trigger-drop`.

### 7. `POST /publish` or `/api/publish`
*   **Description**: Finalizes and publishes drafts.
*   **Payload**: `{ "id": 12, "title": "Custom Title", "description": "Episode description." }`
*   **Logic**: Updates the database status to `published` and immediately refreshes the language RSS feed on S3.

---

## Running & Testing the Pipeline

### Running Unit & Integration Tests
Run local Go tests:
```sh
# Fast unit tests only
go test ./...

# Full integration database tests (spins up Docker container automatically)
go test -tags=integration ./...
```

### Triggering the Podcast Generation Cycle
You can trigger the pipeline in three ways:
1.  **Dashboard Control**: Open `http://localhost:18080/visualizer` and click the "Trigger New Episode" button.
2.  **API Curl**: POST a manual execution request to the Go proxy:
    ```sh
    curl -X POST http://localhost:18080/trigger
    ```
3.  **n8n Direct Trigger**: Open the n8n editor at `http://localhost:15678/`, find the **VoiceScribe — News Pipeline V4** workflow, and click **Execute Workflow**.

### Smoke Testing WhatsApp Onboarding Webhook
Simulate Twilio form inbound from an Italian user picking Italian (option 2):
```sh
curl -X POST http://localhost:18080/webhook/whatsapp \
  --data-urlencode 'From=whatsapp:+393334445555' \
  --data-urlencode 'Body=2'
```
Response (TwiML):
```xml
<Response><Message>You're set! Here is your latest update. Next drop in 2 hours. http://localhost:18080/minio/voicescribe-bucket/episode_it_latest.mp3</Message></Response>
```

---

## Clean Tear Down
To stop all services and keep database/storage volume caches:
```sh
docker compose down
```
To fully reset local state and wipe Postgres database and Minio S3 bucket storage:
```sh
docker compose down
docker run --rm -v "$PWD:/w" alpine rm -rf /w/_data
```

---

## License & Public Distribution Terms

This project is released under the **NewScriber Source-Available License (Personal & Internal Business Use Only)**. 

Under the terms of this license:
*   **Allowed**: You are free to run, copy, modify, and distribute the Software for **Personal Use** (hobbyist, educational, non-commercial) and **Internal Business Use** (within your own organization's internal workflows).
*   **Prohibited**: You may **not** distribute or sublicense the Software for commercial profit, use it to offer a hosted/managed service (SaaS), or deploy it to compete directly or indirectly with the Licensor.

See the complete terms and legal details in the [LICENSE](./LICENSE) file.
