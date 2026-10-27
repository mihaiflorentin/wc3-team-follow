APP_NAME := wc3-team-follow
DIST_DIR := dist
BUILD_DIR := bin
GO_VERSION := 1.27
MAIN_PKG := .
# Container image built by docker-image; override per build, e.g.
# `make docker-image IMAGE_TAG=v1.2.3`.
IMAGE_REPOSITORY ?= wc3-team-follow
IMAGE_TAG ?= latest

# Release metadata, injected into the CLI version variables via -ldflags -X.
# Override on the command line, e.g. `make build-all VERSION=v1.2.3`.
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
VERSION_PKG := github.com/mihaiflorentin/wc3-team-follow/cmd/cli
LDFLAGS := -s -w -X $(VERSION_PKG).version=$(VERSION) -X $(VERSION_PKG).commit=$(COMMIT) -X $(VERSION_PKG).date=$(DATE)
GO_BUILD := CGO_ENABLED=0 go build -trimpath -ldflags '$(LDFLAGS)'

.PHONY: build clean build-all \
	build-linux-amd64 build-linux-arm64 \
	build-darwin-amd64 build-darwin-arm64 \
	build-windows-amd64 build-windows-arm64 \
	docker-build docker-image test tidy fmt lint

build:
	@echo "==> building $(APP_NAME)"
	@mkdir -p $(BUILD_DIR)
	@$(GO_BUILD) -o $(BUILD_DIR)/$(APP_NAME) $(MAIN_PKG)

clean:
	@echo "==> cleaning build artifacts"
	@rm -rf $(BUILD_DIR) $(DIST_DIR)

build-all: clean \
	build-linux-amd64 build-linux-arm64 \
	build-darwin-amd64 build-darwin-arm64 \
	build-windows-amd64 build-windows-arm64

build-linux-amd64:
	@echo "==> linux/amd64"
	@mkdir -p $(DIST_DIR)
	@GOOS=linux GOARCH=amd64 $(GO_BUILD) -o $(DIST_DIR)/$(APP_NAME)-linux-amd64 $(MAIN_PKG)

build-linux-arm64:
	@echo "==> linux/arm64"
	@mkdir -p $(DIST_DIR)
	@GOOS=linux GOARCH=arm64 $(GO_BUILD) -o $(DIST_DIR)/$(APP_NAME)-linux-arm64 $(MAIN_PKG)

build-darwin-amd64:
	@echo "==> darwin/amd64"
	@mkdir -p $(DIST_DIR)
	@GOOS=darwin GOARCH=amd64 $(GO_BUILD) -o $(DIST_DIR)/$(APP_NAME)-darwin-amd64 $(MAIN_PKG)

build-darwin-arm64:
	@echo "==> darwin/arm64"
	@mkdir -p $(DIST_DIR)
	@GOOS=darwin GOARCH=arm64 $(GO_BUILD) -o $(DIST_DIR)/$(APP_NAME)-darwin-arm64 $(MAIN_PKG)

build-windows-amd64:
	@echo "==> windows/amd64"
	@mkdir -p $(DIST_DIR)
	@GOOS=windows GOARCH=amd64 $(GO_BUILD) -o $(DIST_DIR)/$(APP_NAME)-windows-amd64.exe $(MAIN_PKG)

build-windows-arm64:
	@echo "==> windows/arm64"
	@mkdir -p $(DIST_DIR)
	@GOOS=windows GOARCH=arm64 $(GO_BUILD) -o $(DIST_DIR)/$(APP_NAME)-windows-arm64.exe $(MAIN_PKG)

docker-build:
	@echo "==> building inside Docker (golang:$(GO_VERSION))"
	@mkdir -p $(DIST_DIR)
	@docker run --rm \
		-v $$(pwd):/src \
		-w /src \
		golang:$(GO_VERSION) \
		/bin/bash -c "GOOS=linux GOARCH=amd64 $(GO_BUILD) -o $(DIST_DIR)/$(APP_NAME)-linux-amd64 $(MAIN_PKG)"

docker-image:
	@echo "==> building runtime Docker image $(IMAGE_REPOSITORY):$(IMAGE_TAG)"
	@docker build -t $(IMAGE_REPOSITORY):$(IMAGE_TAG) .

test:
	@go test ./...

tidy:
	@go mod tidy

fmt:
	@gofmt -w $$(find . -name '*.go' -not -path './vendor/*')

lint:
	@golangci-lint run ./...
