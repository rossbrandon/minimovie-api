.PHONY: start watch build sync build-sync fmt lint test test-cover local-up local-down

start:
	@set -a && source .env && go run cmd/api/main.go

watch:
	@set -a && source .env && air

build:
	go build -o bin/api cmd/api/main.go

sync:
	@set -a && source .env && go run cmd/sync/main.go $(if $(START),-start=$(START)) $(if $(END),-end=$(END))

build-sync:
	go build -o bin/sync cmd/sync/main.go

fmt:
	go fmt ./...

lint:
	golangci-lint run ./...

test:
	go test ./...

test-cover:
	go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out

local-up:
	docker compose -f local-development/docker-compose.yml up -d

local-down:
	docker compose -f local-development/docker-compose.yml down

