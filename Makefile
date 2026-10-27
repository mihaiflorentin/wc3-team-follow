APP_NAME := wc3-team-follow
APP_BUNDLE := Warcraft Team Follow
DIST_DIR := dist
BUILD_DIR := bin
MAIN_PKG := .

# Release metadata, injected into the CLI version variables via -ldflags -X.
# Override on the command line, e.g. `make package-all VERSION=v1.2.3`.
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
VERSION_PKG := github.com/mihaiflorentin/wc3-team-follow/cmd/cli
LDFLAGS := -s -w -X $(VERSION_PKG).version=$(VERSION) -X $(VERSION_PKG).commit=$(COMMIT) -X $(VERSION_PKG).date=$(DATE)

# Windows resources (icon, manifest, version) need a numeric X.Y.Z version.
WIN_VERSION := $(shell echo '$(VERSION)' | sed -E 's/^v//; s/^([0-9]+\.[0-9]+\.[0-9]+).*/\1/; /^[0-9]+\.[0-9]+\.[0-9]+$$/!s/.*/0.0.0/')
GO_WINRES := go run github.com/tc-hib/go-winres@v0.3.3

# Wails builds the window only with these tags. Linux uses WebKitGTK 4.1.
TAGS := desktop,production
LINUX_TAGS := $(TAGS),webkit2_41

# The window needs cgo on macOS and Linux, so those binaries are built on
# their own OS (see .github/workflows/release.yml). Windows needs no cgo
# and cross-compiles from anywhere.
HOST_OS := $(shell go env GOOS)
HOST_TAGS := $(TAGS)
ifeq ($(HOST_OS),darwin)
# Go 1.27 supports macOS 13 and later.
export MACOSX_DEPLOYMENT_TARGET := 13.0
export CGO_CFLAGS := -mmacosx-version-min=13.0
export CGO_LDFLAGS := -framework UniformTypeIdentifiers -mmacosx-version-min=13.0
endif
ifeq ($(HOST_OS),linux)
HOST_TAGS := $(LINUX_TAGS)
endif

.PHONY: build run clean test vet fmt lint tidy icons winres \
	build-windows-amd64 build-linux-amd64 build-darwin-arm64 \
	package-windows-amd64 package-linux-amd64 package-darwin-arm64

## build: build the app for this computer into bin/
build:
	@echo "==> building $(APP_NAME) for $(HOST_OS)"
	@mkdir -p $(BUILD_DIR)
	@CGO_ENABLED=1 go build -trimpath -tags '$(HOST_TAGS)' -ldflags '$(LDFLAGS)' -o $(BUILD_DIR)/$(APP_NAME) $(MAIN_PKG)

run: build
	@./$(BUILD_DIR)/$(APP_NAME)

## winres: compile the Windows icon, manifest and version into a .syso that
## go build links into the .exe.
winres:
	@$(GO_WINRES) make --in packaging/windows/winres.json --out rsrc --arch amd64 \
		--file-version $(WIN_VERSION).0 --product-version $(WIN_VERSION).0

build-windows-amd64: winres
	@echo "==> windows/amd64"
	@mkdir -p $(DIST_DIR)
	@GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -tags '$(TAGS)' \
		-ldflags '$(LDFLAGS) -H windowsgui' -o $(DIST_DIR)/$(APP_NAME)-windows-amd64.exe $(MAIN_PKG)

build-linux-amd64:
	@echo "==> linux/amd64 (run on linux/amd64 with libwebkit2gtk-4.1-dev)"
	@mkdir -p $(DIST_DIR)
	@GOOS=linux GOARCH=amd64 CGO_ENABLED=1 go build -trimpath -tags '$(LINUX_TAGS)' \
		-ldflags '$(LDFLAGS)' -o $(DIST_DIR)/$(APP_NAME)-linux-amd64 $(MAIN_PKG)

build-darwin-arm64:
	@echo "==> darwin/arm64 (run on macOS)"
	@mkdir -p $(DIST_DIR)
	@GOOS=darwin GOARCH=arm64 CGO_ENABLED=1 go build -trimpath -tags '$(TAGS)' \
		-ldflags '$(LDFLAGS)' -o $(DIST_DIR)/$(APP_NAME)-darwin-arm64 $(MAIN_PKG)

## package-*: build a release file into dist/
package-windows-amd64: build-windows-amd64
	@cd $(DIST_DIR) && rm -f $(APP_NAME)-windows-amd64.zip && zip -q $(APP_NAME)-windows-amd64.zip $(APP_NAME)-windows-amd64.exe

# The Linux tarball carries an icon and a .desktop entry for app launchers.
package-linux-amd64: build-linux-amd64
	@rm -rf $(DIST_DIR)/linux && mkdir -p $(DIST_DIR)/linux
	@cp $(DIST_DIR)/$(APP_NAME)-linux-amd64 $(DIST_DIR)/linux/$(APP_NAME)
	@cp packaging/icon/icon-512.png $(DIST_DIR)/linux/$(APP_NAME).png
	@cp packaging/linux/$(APP_NAME).desktop $(DIST_DIR)/linux/
	@tar -czf $(DIST_DIR)/$(APP_NAME)-linux-amd64.tar.gz -C $(DIST_DIR)/linux .

# The macOS release is an app bundle, ad-hoc signed with the debugger
# entitlement so it may read the game's menu session (the game is built
# with get-task-allow). It is not notarized.
package-darwin-arm64: build-darwin-arm64
	@echo "==> $(APP_BUNDLE).app"
	@rm -rf "$(DIST_DIR)/$(APP_BUNDLE).app"
	@mkdir -p "$(DIST_DIR)/$(APP_BUNDLE).app/Contents/MacOS" "$(DIST_DIR)/$(APP_BUNDLE).app/Contents/Resources"
	@cp packaging/icon/AppIcon.icns "$(DIST_DIR)/$(APP_BUNDLE).app/Contents/Resources/"
	@cp $(DIST_DIR)/$(APP_NAME)-darwin-arm64 "$(DIST_DIR)/$(APP_BUNDLE).app/Contents/MacOS/$(APP_NAME)"
	@sed -e 's/__VERSION__/$(patsubst v%,%,$(VERSION))/g' packaging/darwin/Info.plist > "$(DIST_DIR)/$(APP_BUNDLE).app/Contents/Info.plist"
	@codesign --force --sign - --entitlements packaging/darwin/entitlements.plist "$(DIST_DIR)/$(APP_BUNDLE).app"
	@cd $(DIST_DIR) && rm -f $(APP_NAME)-macos-arm64.zip && ditto -c -k --keepParent "$(APP_BUNDLE).app" $(APP_NAME)-macos-arm64.zip

clean:
	@echo "==> cleaning build artifacts"
	@rm -rf $(BUILD_DIR) $(DIST_DIR) rsrc_windows_*.syso

## icons: re-render every icon from packaging/icon/*.svg (macOS, with
## rsvg-convert, ImageMagick and iconutil). The results are committed.
icons:
	@cd packaging/icon && \
	for s in 16 24 32; do rsvg-convert -w $$s -h $$s icon-small.svg -o win-$$s.png; done && \
	for s in 48 64 128 256 512; do rsvg-convert -w $$s -h $$s icon.svg -o win-$$s.png; done && \
	magick win-16.png win-24.png win-32.png win-48.png win-64.png win-128.png win-256.png icon.ico && \
	cp win-512.png icon-512.png && cp icon-512.png ../../cmd/gui/icon.png && \
	mkdir -p AppIcon.iconset && \
	for s in 16 32 128 256 512; do \
		rsvg-convert -w $$s -h $$s icon-macos.svg -o AppIcon.iconset/icon_$${s}x$${s}.png; \
		rsvg-convert -w $$((s*2)) -h $$((s*2)) icon-macos.svg -o AppIcon.iconset/icon_$${s}x$${s}@2x.png; \
	done && \
	iconutil -c icns AppIcon.iconset -o AppIcon.icns && rm -rf AppIcon.iconset win-*.png

test:
	@go test ./...

vet:
	@go vet ./...

tidy:
	@go mod tidy

fmt:
	@gofmt -w $$(find . -name '*.go' -not -path './vendor/*')

lint:
	@golangci-lint run ./...
