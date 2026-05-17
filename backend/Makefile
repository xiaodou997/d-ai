SHELL := /bin/bash

GO ?= go
CGO_ENABLED ?= 0
GOOS ?= $(shell $(GO) env GOOS)
GOARCH ?= $(shell $(GO) env GOARCH)
VERSION ?= dev

DIST_DIR := dist
BIN_DIR  := $(DIST_DIR)/bin
PKG_DIR  := $(DIST_DIR)/packages
APP_NAME := uni-ai-api-backend-$(VERSION)-$(GOOS)-$(GOARCH)
BINS     := server fake-upstream

.PHONY: dev build

dev:
	APP_ENV=development LOG_LEVEL=debug $(GO) run ./cmd/server

build:
	@mkdir -p $(BIN_DIR) $(PKG_DIR)
	@for bin in $(BINS); do \
		CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) \
		$(GO) build -trimpath -ldflags "-X main.version=$(VERSION)" \
		-o $(BIN_DIR)/$$bin ./cmd/$$bin; \
	done
	@rm -rf $(DIST_DIR)/release
	@mkdir -p $(DIST_DIR)/release/$(APP_NAME)/bin
	@cp $(BIN_DIR)/* $(DIST_DIR)/release/$(APP_NAME)/bin/
	@cp -R db $(DIST_DIR)/release/$(APP_NAME)/
	@cp config.example.yaml $(DIST_DIR)/release/$(APP_NAME)/
	@tar -C $(DIST_DIR)/release -czf $(PKG_DIR)/$(APP_NAME).tar.gz $(APP_NAME)
	@echo "build: $(PKG_DIR)/$(APP_NAME).tar.gz"
