.PHONY: start build fmt

start:
	@set -a && source .env && go run cmd/api/main.go

build:
	go build -o bin/api cmd/api/main.go

fmt:
	go fmt ./...

