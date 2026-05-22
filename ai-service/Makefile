SHELL := /bin/bash

GO ?= go
CGO_ENABLED ?= 0
VERSION ?= dev

.PHONY: dev build clean

dev:
	APP_ENV=development LOG_LEVEL=debug $(GO) run ./cmd/server

build:
	@echo "正在构建 ai-service (Linux amd64)..."
	@CGO_ENABLED=$(CGO_ENABLED) GOOS=linux GOARCH=amd64 \
		$(GO) build -trimpath -ldflags="-s -w -X main.version=$(VERSION)" \
		-o bin/server-amd64 ./cmd/server
	@echo "构建完成: bin/server-amd64"
	@echo "正在构建 ai-service (Linux arm64)..."
	@CGO_ENABLED=$(CGO_ENABLED) GOOS=linux GOARCH=arm64 \
		$(GO) build -trimpath -ldflags="-s -w -X main.version=$(VERSION)" \
		-o bin/server-arm64 ./cmd/server
	@echo "构建完成: bin/server-arm64"

clean:
	@rm -rf bin
