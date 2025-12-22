.PHONY: start watch build fmt

start:
	@set -a && source .env && go run cmd/api/main.go

watch:
	@set -a && source .env && air

build:
	go build -o bin/api cmd/api/main.go

fmt:
	go fmt ./...

