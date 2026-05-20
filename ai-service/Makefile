SHELL := /bin/bash

GO ?= go
CGO_ENABLED ?= 0
GOOS ?= $(shell $(GO) env GOOS)
GOARCH ?= $(shell $(GO) env GOARCH)
VERSION ?= dev

RELEASE_DIR := release
APP_NAME := uni-ai-api-$(VERSION)-$(GOOS)-$(GOARCH)
BINS := server fake-upstream

.PHONY: dev build clean

dev:
	APP_ENV=development LOG_LEVEL=debug $(GO) run ./cmd/server

build:
	@mkdir -p $(RELEASE_DIR)/$(APP_NAME)/bin
	@for bin in $(BINS); do \
		CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) \
		$(GO) build -trimpath -ldflags "-X main.version=$(VERSION)" \
		-o $(RELEASE_DIR)/$(APP_NAME)/bin/$$bin ./cmd/$$bin; \
	done
	@cp -R db $(RELEASE_DIR)/$(APP_NAME)/
	@cp config.example.yaml $(RELEASE_DIR)/$(APP_NAME)/
	@tar -C $(RELEASE_DIR) -czf $(RELEASE_DIR)/$(APP_NAME).tar.gz $(APP_NAME)
	@echo "build: $(RELEASE_DIR)/$(APP_NAME).tar.gz"

clean:
	@rm -rf $(RELEASE_DIR)
