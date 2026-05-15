# VoiceScribe — Information delivered in local vocal

V1 hackathon prototype of a multilingual automated WhatsApp news delivery system.
This slice is the **interactive onboarding webhook**: receive WhatsApp inbound
messages, register the user in Postgres, let them pick a language
(English / Italian / Bangla), and acknowledge with a placeholder audio drop.

## Stack

- Go service (`cmd/webhook`) — `net/http` + `database/sql`
- Postgres 15 — via `github.com/jackc/pgx/v5/stdlib`
- n8n — for downstream automation
- All orchestrated by Docker Compose under the project name `voicescribe`

## System architecture

### Where we are (V1 — onboarding only)

```
   WhatsApp user
       │  text message
       ▼
  ┌─────────────────────┐
  │ Twilio  /  Meta     │   (provider relays inbound to our webhook;
  │ WhatsApp Cloud API  │    will also be used for outbound in V2)
  └────────┬────────────┘
           │ HTTPS POST  (form-encoded OR JSON)
           ▼
  ┌──────────────────────────────────────────────────┐
  │ webhook-app  (Go, container :8080 → host :18080) │
  │                                                  │
  │   internal/webhook/parser.go                     │
  │     Content-Type → Twilio form  OR  Meta JSON    │
  │                                                  │
  │   internal/webhook/handler.go                    │
  │     ┌─ body == "1" / "2" / "3"  →  upsert lang   │
  │     ├─ unknown user OR "Get News"  →  menu       │
  │     └─ known user, free text  →  reminder        │
  │                                                  │
  │   reply: TwiML (Twilio) or JSON (Meta)           │
  └──────────────┬───────────────────────────────────┘
                 │ database/sql + pgx
                 ▼
  ┌──────────────────────────────────────────────────┐
  │ db  (postgres:15-alpine, :5432 → host :55432)    │
  │   users(phone_number PK, language_pref, ...)     │
  └──────────────────────────────────────────────────┘

  ┌──────────────────────────────────────────────────┐
  │ n8n  (in the stack but not yet wired to flow;    │
  │       reserved for the news pipeline in V2)      │
  └──────────────────────────────────────────────────┘
```

### Where we're going (V2 — closing the loop)

```
                ┌──────────────────────────┐
                │ News sources (RSS / API) │
                └────────────┬─────────────┘
                             │
                             ▼
  ┌────────────────────────────────────────────────────────────┐
  │ n8n  (every ~2h cron workflow)                             │
  │   1. fetch & dedupe stories                                │
  │   2. summarize  (LLM — Anthropic / OpenAI)                 │
  │   3. translate  → en / it / bn                             │
  │   4. text-to-speech  → MP3 per language                    │
  │   5. upload MP3 to object store, capture public URL        │
  │   6. POST /broadcast { language, text, mediaUrl }          │
  └────────────────────────────────┬───────────────────────────┘
                                   │
                                   ▼
  ┌────────────────────────────────────────────────────────────┐
  │ webhook-app                                                │
  │   POST /webhook/whatsapp   inbound  (already exists)       │
  │   POST /broadcast          outbound  (NEW)                 │
  │     → SELECT phone_number FROM users WHERE language=…      │
  │     → Twilio/Meta send-message API with media URL          │
  │     → record row in deliveries(broadcast_id, phone, …)     │
  └──────┬─────────────────────────┬───────────────────────────┘
         │ pgx                     │ HTTPS
         ▼                         ▼
  ┌──────────────────┐   ┌────────────────────────────────┐
  │ Postgres         │   │ Twilio / Meta send-message API │
  │  users           │   └──────────────┬─────────────────┘
  │  broadcasts      │                  │
  │  deliveries      │                  ▼
  └──────────────────┘             WhatsApp user
                                   (audio drop)

  ┌────────────────────────────────────────────────────────────┐
  │ Object store (R2 / S3) — public MP3 URLs                   │
  └────────────────────────────────────────────────────────────┘
```

### Design notes — why these pieces

| Choice                                              | Why                                                                                                                       |
| --------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------- |
| **Go for the webhook**                              | Single static binary, tiny image, `net/http` is enough, instant cold-start, easy to keep running long-term.               |
| **`database/sql` + `pgx/v5/stdlib`**                | Stdlib boundary keeps repo code portable; pgx gives us modern Postgres semantics under the hood.                          |
| **Repo interface + in-memory fake + testcontainers** | Handler tests stay sub-millisecond against a fake; repo SQL stays correct against a real Postgres in a tagged test.       |
| **Two payload shapes (Twilio + Meta) behind one parser** | Lets us swap WhatsApp providers later without touching handler/business logic.                                       |
| **TwiML reply for Twilio, JSON for Meta**           | Twilio responds synchronously in the webhook reply; Meta uses a separate outbound API call — JSON keeps that decoupled.   |
| **n8n in the stack (not yet wired)**                | Visual workflow editor for the news pipeline → non-engineers can iterate on prompt/source/cadence without code deploys.   |
| **Mock MP3 URL in V1**                              | Decouples onboarding from the audio pipeline so we ship onboarding before TTS exists. The URL is the seam.                |
| **`_data/` bind mounts under compose project namespace** | Host-visible data, no Go-package-walker collisions, no container-name collisions with other projects.                |
| **`.env` via Compose `env_file:`**                  | Secrets live outside source; new keys (Twilio token, Meta secret, R2 creds…) flow into the container with no compose edit. |

## Run tests locally

Unit tests only (fast, no Docker required):

```sh
go test ./...
```

Repository integration tests against a real Postgres
(spins a container via `testcontainers-go` — needs Docker available):

```sh
go test -tags=integration ./...
```

## Bring up the full stack

```sh
docker compose up --build -d
```

All resources are namespaced under the compose project `voicescribe`
(containers: `voicescribe-db`, `voicescribe-webhook`, `voicescribe-n8n`;
network: `voicescribe-net`). Host-side ports are intentionally non-default
to avoid colliding with any other Postgres / web app / n8n already
running on the machine:

| Service       | Host port | Container port | Notes                                                          |
| ------------- | --------- | -------------- | -------------------------------------------------------------- |
| `db`          | `55432`   | `5432`         | Postgres 15, data persisted to `./_data/postgres`              |
| `webhook-app` | `18080`   | `8080`         | Go webhook, reads `DATABASE_URL`, runs migrations on boot      |
| `n8n`         | `15678`   | `5678`         | Workflow engine, data persisted to `./_data/n8n`               |

> **Why `_data/`?** Go's `./...` package walker ignores directories starting
> with `_` or `.`, so the host-side data dirs (which contain root-owned
> Postgres files) don't break `go test ./...`.

Health check:

```sh
curl http://localhost:18080/healthz
# → ok
```

Compose-side, `webhook-app` also has a container healthcheck that runs the
binary itself with `-healthcheck` against `127.0.0.1:8080/healthz` — `docker
compose ps` will show `(healthy)` once it's serving.

> **Note on the n8n bind mount:** n8n runs as UID 1000 inside the container
> and needs to own `./_data/n8n` on the host. If you ever wipe and recreate
> that directory, fix ownership with:
> ```sh
> docker run --rm -v "$PWD/_data:/d" alpine chown -R 1000:1000 /d/n8n
> ```

## Smoke test the webhook

**Twilio-style (form-encoded) inbound — new user asking for the menu:**

```sh
curl -X POST http://localhost:18080/webhook/whatsapp \
  --data-urlencode 'From=whatsapp:+391112223333' \
  --data-urlencode 'Body=Get News'
```

Reply (TwiML):

```xml
<Response><Message>Welcome to VoiceScribe. Choose your language: ...</Message></Response>
```

**Pick Italian (option 2):**

```sh
curl -X POST http://localhost:18080/webhook/whatsapp \
  --data-urlencode 'From=whatsapp:+391112223333' \
  --data-urlencode 'Body=2'
```

Reply confirms with the placeholder MP3 URL. The user's
`language_pref` in Postgres is now `it`:

```sh
docker compose exec db psql -U voicescribe -d voicescribe -c 'SELECT * FROM users;'
```

(or from the host: `psql -h localhost -p 55432 -U voicescribe -d voicescribe`)

**Meta Cloud API-style (JSON) inbound:**

```sh
curl -X POST http://localhost:18080/webhook/whatsapp \
  -H 'Content-Type: application/json' \
  -d '{"entry":[{"changes":[{"value":{"messages":[{"from":"391112223333","text":{"body":"1"}}]}}]}]}'
```

Reply is JSON: `{"reply":"You're set! ..."}`.

## Production setup

This compose stack is built to be promoted to a production-like deployment
with **only configuration changes** — no code changes. The hardening already in
place:

| Concern                  | What's in place                                                                                           |
| ------------------------ | --------------------------------------------------------------------------------------------------------- |
| Secrets out of source    | `${VAR:-default}` interpolation throughout `docker-compose.yml`. Override via a `.env` file (gitignored). |
| Container restart        | `restart: unless-stopped` on every service.                                                               |
| DB readiness gate        | `db` has a `pg_isready` healthcheck; `webhook-app` waits for `service_healthy` before starting.           |
| App healthcheck          | Webhook binary supports `-healthcheck`; container healthcheck uses it (no need to bundle `curl`).         |
| Container hardening      | `webhook-app` runs `read_only: true`, `cap_drop: ALL`, `no-new-privileges`, `USER nobody`.                |
| Structured logging       | Go service uses `log/slog` (JSON by default). Tunable via `LOG_LEVEL` / `LOG_FORMAT`.                     |
| Request observability    | All HTTP requests logged with method/path/status/duration.                                                |
| DB pool tuning           | `SetMaxOpenConns=25`, `SetMaxIdleConns=5`, `SetConnMaxLifetime=5m`, `SetConnMaxIdleTime=2m`.              |
| Image size               | Multi-stage build, CGO off, alpine runner — final image is small.                                         |

### Deploying for real — the playbook

1. **Copy `.env.example` → `.env`** and set strong values for
   `POSTGRES_PASSWORD` (the example default is a placeholder).
2. **Front the webhook with TLS.** WhatsApp/Twilio require HTTPS. In V1
   the Go service speaks plain HTTP; in production terminate TLS at a
   reverse proxy (Caddy / Traefik / nginx / cloud LB) pointing at
   `webhook-app:8080`.
3. **Restrict the n8n and Postgres host ports.** Edit the `ports:` lines
   to bind to `127.0.0.1` so they're not reachable from the LAN:
   ```yaml
   ports:
     - "127.0.0.1:55432:5432"   # db
     - "127.0.0.1:15678:5678"   # n8n
   ```
4. **Complete the n8n owner-account setup.** Hit
   `http://localhost:15678/setup` on first launch and create the owner
   account. Current n8n versions deprecated `N8N_BASIC_AUTH_*` in favor of
   this flow.
5. **Back up `./_data/postgres`** (the bind-mount holds all DB state).
6. **Plan signature verification** for the webhook (see follow-ups below).

## Roadmap — what's left to reach the full goal

Half of the product — inbound onboarding — exists. The other half — outbound
audio news drops — is missing. Milestones below are listed in build order;
M1 unblocks everything downstream.

### M1 — Outbound delivery (the missing half)

- [ ] `internal/notify`: Twilio + Meta send-message clients (text + media)
- [ ] `POST /broadcast` on `webhook-app`, accepting `{ language, text, mediaUrl }`
- [ ] New tables: `broadcasts(id, language, text, mp3_url, created_at)` and
      `deliveries(broadcast_id, phone, status, provider_msg_id, error, sent_at)`
- [ ] `.env` keys for `TWILIO_ACCOUNT_SID`, `TWILIO_AUTH_TOKEN`,
      `WHATSAPP_FROM`, or `META_PHONE_NUMBER_ID` / `META_ACCESS_TOKEN`
- [ ] Idempotency on broadcast: same `(broadcast_id, phone)` never sends twice

### M2 — News pipeline (n8n workflows)

- [ ] Pick news sources (RSS for hackathon; AP / Reuters / NewsAPI for prod)
- [ ] Workflow: cron `0 */2 * * *` → fetch → dedupe via Postgres → top N stories
- [ ] LLM summarization node → ~60-90 sec spoken script
- [ ] Translation: en → it, en → bn (LLM or DeepL)
- [ ] TTS per language (ElevenLabs / Azure Speech / Google) → MP3
- [ ] Upload to object store, capture public URL
- [ ] HTTP node calls back into `webhook-app /broadcast`

### M3 — Media hosting

- [ ] Choose: Cloudflare R2 / AWS S3 / local volume + sidecar nginx
- [ ] Verify the URL is GET-able by Twilio/Meta media-fetching IPs
- [ ] Lifecycle policy (drop MP3s older than ~7 days to control cost)

### M4 — Webhook security

- [ ] Twilio: validate `X-Twilio-Signature` HMAC with auth token before trusting `From`/`Body`
- [ ] Meta: validate `X-Hub-Signature-256` with the app secret
- [ ] Meta: `GET /webhook/whatsapp` verify-token handshake
- [ ] Per-phone rate limit at the edge (e.g. 1 inbound per 2s)

### M5 — Production exposure

- [ ] TLS termination via reverse proxy (Caddy / Traefik) — WhatsApp requires HTTPS
- [ ] Public DNS name for the webhook
- [ ] Versioned migrations (`golang-migrate`) — current `migrate.go` is single-statement idempotent
- [ ] CI: `go test ./...` + integration on push; build & push image on tag
- [ ] Backup `_data/postgres` (pg_dump cron)
- [ ] Metrics: Prometheus or OTel → Grafana (request rate, broadcast fanout, send failures)

### M6 — UX polish

- [ ] Localize the welcome menu and reply strings per `language_pref`
- [ ] `STOP` / `PAUSE` keyword → opt-out flag on user
- [ ] `LANG` / `CHANGE` keyword → re-show menu without re-onboarding
- [ ] Time-of-day preference (only send during user's daytime)

### M7 — Scale & cost

- [ ] Cache TTS output per (story × language) — don't regenerate per user
- [ ] WhatsApp **template messages** for first-touch outbound where required
- [ ] Per-user delivery throttling to stay under provider rate limits
- [ ] Cost dashboards: TTS minutes, LLM tokens, WhatsApp message units

## Tear down

```sh
docker compose down
# fully reset local state (Postgres data + n8n config):
docker run --rm -v "$PWD:/w" alpine rm -rf /w/_data
```

## Project layout

```
cmd/webhook/main.go                       entrypoint: env → db → repo → handler → http.Server
                                          + slog setup, -healthcheck flag, request logger
internal/db/migrate.go                    CREATE TABLE IF NOT EXISTS users (...)
internal/users/repo.go                    UserRepository interface + PgUserRepository
internal/users/fake.go                    InMemoryUserRepository for handler tests
internal/users/repo_integration_test.go   testcontainers-go Postgres test (build tag: integration)
internal/webhook/parser.go                ParseInbound — Twilio form OR Meta JSON
internal/webhook/handler.go               POST /webhook/whatsapp logic + TwiML/JSON reply
.env.example                              committed template for production overrides
docker-compose.yml                        namespaced under `voicescribe` project
Dockerfile                                multi-stage, CGO off, alpine runner
```
