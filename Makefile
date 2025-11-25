.PHONY: help up down logs ps migrate-up migrate-down migrate-create clean build run test lint

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

up: ## Start all services
	docker compose up -d

down: ## Stop all services
	docker compose down

logs: ## Tail logs for all services
	docker compose logs -f

ps: ## Show running services
	docker compose ps

migrate-up: ## Run database migrations
	docker compose run --rm migrate -path=/migrations -database=postgres://gostream:gostream@postgres:5432/gostream?sslmode=disable up

migrate-down: ## Rollback last migration
	docker compose run --rm migrate -path=/migrations -database=postgres://gostream:gostream@postgres:5432/gostream?sslmode=disable down 1

migrate-create: ## Create new migration (usage: make migrate-create NAME=create_users)
	@if [ -z "$(NAME)" ]; then echo "Usage: make migrate-create NAME=migration_name"; exit 1; fi
	@NEXT_NUM=$$(ls -1 db/migrations/*.up.sql 2>/dev/null | wc -l | tr -d ' '); \
	NEXT_NUM=$$(printf "%06d" $$((NEXT_NUM + 1))); \
	touch db/migrations/$${NEXT_NUM}_$(NAME).up.sql db/migrations/$${NEXT_NUM}_$(NAME).down.sql; \
	echo "Created db/migrations/$${NEXT_NUM}_$(NAME).up.sql"; \
	echo "Created db/migrations/$${NEXT_NUM}_$(NAME).down.sql"

clean: ## Remove all docker data (WARNING: destructive)
	docker compose down -v
	rm -rf .docker-data

build: ## Build Go binaries
	go build -o bin/api ./cmd/api

run: ## Run API server locally
	go run ./cmd/api

test: ## Run tests
	go test -v -race ./...

lint: ## Run linter
	golangci-lint run ./...
