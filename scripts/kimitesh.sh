#!/bin/bash
# ─────────────────────────────────────────────
# Azure OpenAI – Kimi-K2.5 quick test (curl)
# Endpoint: https://devel-mnww3344-eastus2.openai.azure.com/
# ─────────────────────────────────────────────

AZURE_ENDPOINT="https://devel-mnww3344-eastus2.openai.azure.com"
DEPLOYMENT_NAME="Kimi-K2.5"           # change if your deployment name differs
API_VERSION="2024-12-01-preview"       # latest GA version that supports chat
API_KEY="${AZURE_OPENAI_API_KEY:-}"    # export AZURE_OPENAI_API_KEY=<your-key>

# ── Guard ────────────────────────────────────
if [[ -z "$API_KEY" ]]; then
  echo "ERROR: Set your key first:"
  echo "  export AZURE_OPENAI_API_KEY=<your-api-key>"
  exit 1
fi

URL="${AZURE_ENDPOINT}/openai/deployments/${DEPLOYMENT_NAME}/chat/completions?api-version=${API_VERSION}"

echo "────────────────────────────────────────"
echo " Calling: $URL"
echo "────────────────────────────────────────"

curl -s -X POST "$URL" \
  -H "Content-Type: application/json" \
  -H "api-key: $API_KEY" \
  -d '{
    "messages": [
      {
        "role": "system",
        "content": "You are a helpful assistant."
      },
      {
        "role": "user",
        "content": "Say hello and tell me which model you are in one sentence."
      }
    ],
    "max_tokens": 128,
    "temperature": 0.7
  }' | python3 -m json.tool   # pretty-print; remove if python3 unavailable

echo ""
echo "Done."
