.PHONY: start watch build fmt local-up local-down

start:
	@set -a && source .env && go run cmd/api/main.go

watch:
	@set -a && source .env && air

build:
	go build -o bin/api cmd/api/main.go

fmt:
	go fmt ./...

local-up:
	docker compose -f local-development/docker-compose.yml up -d

local-down:
	docker compose -f local-development/docker-compose.yml down

