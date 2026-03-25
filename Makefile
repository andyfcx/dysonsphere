.PHONY: dev server agent web test migrate-up migrate-down seed lint build clean

# ── local dev ─────────────────────────────────────────────────────────────────

dev: ## Start all services via docker-compose
	docker compose up --build

dev-db: ## Start only postgres
	docker compose up postgres

dev-server: ## Run server locally (requires postgres)
	cd apps/server && go run ./cmd/server

dev-agent: ## Run agent locally
	cd apps/agent && go run ./cmd/agent run --config infra/examples/agent-config.yaml

dev-web: ## Start frontend dev server
	cd apps/web && npm run dev

# ── build ─────────────────────────────────────────────────────────────────────

build: build-server build-agent build-web ## Build all

build-server:
	cd apps/server && go build -o ../../bin/observer-server ./cmd/server

build-agent:
	cd apps/agent && go build -o ../../bin/observer-agent ./cmd/agent

build-web:
	cd apps/web && npm run build

# ── test ──────────────────────────────────────────────────────────────────────

test: test-server test-agent ## Run all Go tests

test-server:
	cd apps/server && go test ./...

test-agent:
	cd apps/agent && go test ./...

test-verbose:
	cd apps/server && go test -v ./... && cd ../agent && go test -v ./...

# ── database ──────────────────────────────────────────────────────────────────

migrate-up: ## Apply all pending migrations
	cd apps/server && go run ./cmd/server migrate up

migrate-down: ## Rollback last migration
	cd apps/server && go run ./cmd/server migrate down

migrate-create: ## Create new migration: make migrate-create NAME=add_foo
	migrate create -ext sql -dir apps/server/migrations -seq $(NAME)

seed: ## Insert sample data
	docker compose --profile seed up seed

# ── code quality ──────────────────────────────────────────────────────────────

lint:
	cd apps/server && go vet ./... && cd ../agent && go vet ./...

sqlc-gen: ## Regenerate sqlc
	cd apps/server && sqlc generate

tidy: ## Tidy all go modules
	cd apps/server && go mod tidy
	cd apps/agent && go mod tidy
	cd packages/shared && go mod tidy
	go work sync

# ── agent helpers ─────────────────────────────────────────────────────────────

agent-init: ## Initialize agent against local server
	cd apps/agent && go run ./cmd/agent init \
		--server http://localhost:8000 \
		--token dev-token \
		--env dev \
		--tags local,test

clean:
	rm -rf bin/
	cd apps/web && rm -rf dist/
