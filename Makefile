.PHONY: help up down logs ps migrate-up migrate-down migrate-create clean build run test lint \
	loadtest-up loadtest-down loadtest-setup loadtest-viral loadtest-clear-cache loadtest-check-db

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

# =============================================================================
# Load Testing
# =============================================================================

loadtest-up: ## Start load test environment (InfluxDB + Grafana + Prometheus)
	docker compose --profile loadtest up -d influxdb grafana prometheus
	@echo "Grafana: http://localhost:3001 (admin/admin)"
	@echo "InfluxDB: http://localhost:8086"
	@echo "Prometheus: http://localhost:9099"

loadtest-down: ## Stop load test environment
	docker compose --profile loadtest down

loadtest-setup: ## Setup test data for load testing
	./tests/load/scripts/setup-test-data.sh

loadtest-viral: ## Run Scenario A: Viral Video (Singleflight test)
	@if [ -f .loadtest.env ]; then . ./.loadtest.env; fi && \
	docker compose --profile loadtest run --rm \
		-e TEST_VIDEO_ID="$${TEST_VIDEO_ID}" \
		k6 run --out influxdb=http://influxdb:8086/k6 /tests/scenarios/scenario-a-viral.js

loadtest-clear-cache: ## Clear Redis and Nginx caches
	./tests/load/scripts/clear-caches.sh

loadtest-check-db: ## Check DB query statistics (pg_stat_statements)
	./tests/load/scripts/check-db-queries.sh

loadtest-check-db-reset: ## Reset DB query statistics before load test
	./tests/load/scripts/check-db-queries.sh --reset
