#!/usr/bin/env bash

# This script truncates the primary data tables in the PostgreSQL database
# and resets their identity (primary key) sequences to 0.
# It is useful for getting a clean state before running the n8n pipeline.

set -e

echo "Cleaning up the VoiceScribe database..."

docker compose exec -T db psql -U voicescribe -d voicescribe -c "TRUNCATE TABLE news_items, episodes RESTART IDENTITY CASCADE;"

echo "Database cleanup complete! All news items and episodes have been deleted."
