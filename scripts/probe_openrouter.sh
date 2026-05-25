#!/bin/bash
# Three OpenRouter probes from inside the webhook container:
#   1. /auth/key          — minimal "is this key valid" check
#   2. /models            — public, no auth strictly required
#   3. /audio/speech      — exact docs example for the failing call
set -u
cd "$(dirname "$0")/.."

KEY=$(grep '^OPENROUTER_API_KEY=' .env | head -1 | cut -d= -f2-)
echo "host_key_len=${#KEY} prefix=${KEY:0:12}"

docker compose exec -T -e PROBE_KEY="$KEY" webhook-app sh <<'EOS'
echo "===[1] /auth/key (key validity)==="
wget -S --header="Authorization: Bearer $PROBE_KEY" -O - \
  https://openrouter.ai/api/v1/auth/key 2>&1 | head -20

echo "===[2] /models (no-auth-needed sanity)==="
wget -S -O - https://openrouter.ai/api/v1/models 2>&1 | head -5

echo "===[3] /audio/speech docs example==="
wget -S \
  --header="Content-Type: application/json" \
  --header="Authorization: Bearer $PROBE_KEY" \
  --post-data='{"model":"google/gemini-3.1-flash-tts-preview","input":"Hello!","voice":"Zephyr","response_format":"pcm"}' \
  -O /dev/shm/out.pcm \
  https://openrouter.ai/api/v1/audio/speech 2>&1 | head -15
EOS
