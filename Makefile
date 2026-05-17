# .PHONY ensures that targets are treated as commands, not files
.PHONY: help up down restart logs ps build n8n-shell db-shell webhook-logs n8n-logs deploy

# Remote deployment variables
REMOTE_HOST = 34.158.35.91
REMOTE_USER = developer
SSH_KEY = newscribe.pem
REMOTE_DIR = ~/hackathon-lab

# Default target: show help
help:
	@echo "VoiceScribe Management Commands:"
	@echo "  make up           - Start all services in the background"
	@echo "  make down         - Stop and remove all containers"
	@echo "  make restart      - Restart all services"
	@echo "  make logs         - Follow logs from all services"
	@echo "  make n8n-logs     - Follow logs from the n8n service"
	@echo "  make webhook-logs - Follow logs from the webhook-app service"
	@echo "  make ps           - List running containers"
	@echo "  make build        - Rebuild the webhook-app image"
	@echo "  make n8n-shell    - Open a shell in the n8n container"
	@echo "  make db-shell     - Open a psql shell in the database container"
	@echo "  make deploy       - Deploy updates to the remote server"

# Start all services
up:
	docker compose --profile local up -d

# Stop and remove all containers
down:
	docker compose down

# Restart all services
restart:
	docker compose restart

# Follow logs from all services
logs:
	docker compose logs -f

# Follow logs from n8n
n8n-logs:
	docker compose logs -f n8n

# Follow logs from webhook-app
webhook-logs:
	docker compose logs -f webhook-app

# List containers
ps:
	docker compose ps

# Rebuild images
build:
	docker compose build

# Shell into n8n container
n8n-shell:
	docker compose exec n8n sh

# Shell into postgres container
db-shell:
	docker compose exec db psql -U voicescribe -d voicescribe

# Deploy to remote server
deploy:
	@echo "Deploying to $(REMOTE_USER)@$(REMOTE_HOST)..."
	# Sync files excluding data and local-only files
	rsync -avz -e "ssh -i $(SSH_KEY) -o StrictHostKeyChecking=no" \
		--exclude '_data/' \
		--exclude '.git/' \
		--exclude '.agent/' \
		--exclude '.claude/' \
		--exclude '.firecrawl/' \
		--exclude 'newscribe.pem' \
		--exclude '.env' \
		--exclude '.env.production' \
		./ $(REMOTE_USER)@$(REMOTE_HOST):$(REMOTE_DIR)/
	# Push production-specific .env
	scp -i $(SSH_KEY) -o StrictHostKeyChecking=no .env.production $(REMOTE_USER)@$(REMOTE_HOST):$(REMOTE_DIR)/.env
	# Trigger remote docker-compose up
	ssh -i $(SSH_KEY) -o StrictHostKeyChecking=no $(REMOTE_USER)@$(REMOTE_HOST) \
		"cd $(REMOTE_DIR) && docker compose up -d --build"
