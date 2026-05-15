#!/bin/sh
set -e

echo "Setting up n8n dependencies..."

# Generate temporary credentials file from environment variables
cat <<EOF > /tmp/n8n-creds.json
[
  {
    "name": "Firecrawl Bearer",
    "type": "httpHeaderAuth",
    "id": "FIRECRAWL_CRED",
    "data": {
      "name": "Authorization",
      "value": "Bearer ${FIRECRAWL_API_KEY}"
    }
  },
  {
    "name": "Azure OpenAI Key",
    "type": "httpHeaderAuth",
    "id": "AZURE_OPENAI_CRED",
    "data": {
      "name": "api-key",
      "value": "${AZURE_OPENAI_API_KEY}"
    }
  },
  {
    "name": "VoiceScribe Postgres",
    "type": "postgres",
    "id": "PG_CRED",
    "data": {
      "host": "db",
      "database": "${POSTGRES_DB:-voicescribe}",
      "user": "${POSTGRES_USER:-voicescribe}",
      "password": "${POSTGRES_PASSWORD:-voicescribe}",
      "port": 5432
    }
  }
]
EOF

echo "Importing credentials..."
n8n import:credentials --input=/tmp/n8n-creds.json || echo "Warning: Some credentials could not be imported (they might already exist)."
rm -f /tmp/n8n-creds.json

echo "Importing workflows..."
if [ -d "/workflows" ]; then
  n8n import:workflow --separate --input=/workflows || echo "Warning: Workflow import failed."
fi

echo "Initialization complete. Starting n8n..."
exec n8n start
