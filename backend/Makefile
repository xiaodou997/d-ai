SHELL := /bin/bash

GO ?= go
CGO_ENABLED ?= 0
GOOS ?= $(shell $(GO) env GOOS)
GOARCH ?= $(shell $(GO) env GOARCH)
VERSION ?= dev
CONFIG ?= config.local.yaml

DIST_DIR := dist
BIN_DIR := $(DIST_DIR)/bin
PKG_DIR := $(DIST_DIR)/packages
APP_NAME := uni-ai-api-backend-$(VERSION)-$(GOOS)-$(GOARCH)
ARCHIVE := $(PKG_DIR)/$(APP_NAME).tar.gz
BINS := server migrate seed fake-upstream

.PHONY: help run run-fake migrate-up migrate-down seed test test-cover build package clean

help:
	@echo "Targets:"
	@echo "  make run         - run backend server (uses UNI_AI_API_CONFIG=$(CONFIG))"
	@echo "  make run-fake    - run fake upstream server"
	@echo "  make migrate-up  - run migrations up"
	@echo "  make migrate-down - run migrations down"
	@echo "  make seed        - run seed sql (optional: SEED_FILE=path.sql)"
	@echo "  make test        - run all unit tests"
	@echo "  make test-cover  - run tests with coverage report"
	@echo "  make build       - build all backend binaries into $(BIN_DIR)"
	@echo "  make package     - package binaries + migrations + config examples"
	@echo "  make clean       - remove $(DIST_DIR)"

run:
	UNI_AI_API_CONFIG=$(CONFIG) $(GO) run ./cmd/server

run-fake:
	$(GO) run ./cmd/fake-upstream

migrate-up:
	UNI_AI_API_CONFIG=$(CONFIG) $(GO) run ./cmd/migrate up

migrate-down:
	UNI_AI_API_CONFIG=$(CONFIG) $(GO) run ./cmd/migrate down

seed:
	UNI_AI_API_CONFIG=$(CONFIG) $(GO) run ./cmd/seed $(SEED_FILE)

test:
	$(GO) test ./...

test-cover:
	$(GO) test ./... -coverprofile=$(DIST_DIR)/coverage.out
	@echo "coverage file: $(DIST_DIR)/coverage.out"

build: $(BINS:%=$(BIN_DIR)/%)

$(BIN_DIR)/%:
	@mkdir -p $(BIN_DIR)
	CGO_ENABLED=$(CGO_ENABLED) GOOS=$(GOOS) GOARCH=$(GOARCH) $(GO) build -trimpath -o $@ ./cmd/$*

package: build
	@mkdir -p $(PKG_DIR)
	@rm -rf $(DIST_DIR)/release
	@mkdir -p $(DIST_DIR)/release/$(APP_NAME)/bin
	@cp $(BIN_DIR)/* $(DIST_DIR)/release/$(APP_NAME)/bin/
	@cp -R migrations $(DIST_DIR)/release/$(APP_NAME)/
	@cp config.example.yaml config.local.example.yaml $(DIST_DIR)/release/$(APP_NAME)/
	@tar -C $(DIST_DIR)/release -czf $(ARCHIVE) $(APP_NAME)
	@echo "package created: $(ARCHIVE)"

clean:
	@rm -rf $(DIST_DIR)
