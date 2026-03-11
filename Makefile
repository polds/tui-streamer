APP_NAME    := TuiStreamer
BINARY_NAME := tui-streamer
VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_DIR   := dist
APP_BUNDLE  := $(BUILD_DIR)/$(APP_NAME).app
DMG_PATH    := $(BUILD_DIR)/$(APP_NAME)-$(VERSION).dmg

# Go build flags
LDFLAGS := -ldflags "-X main.version=$(VERSION) -s -w"

.PHONY: all build build-darwin build-darwin-arm64 build-darwin-amd64 \
        build-darwin-webview app app-server dmg icon clean test lint

all: build

## build: build for the current platform (server binary)
build:
	go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/server

## build-darwin: build a universal (arm64 + amd64) macOS server binary
build-darwin: build-darwin-arm64 build-darwin-amd64
	mkdir -p $(BUILD_DIR)
	lipo -create -output $(BUILD_DIR)/$(BINARY_NAME)-darwin-universal \
		$(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 \
		$(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64
	@echo "Universal binary → $(BUILD_DIR)/$(BINARY_NAME)-darwin-universal"

build-darwin-arm64:
	mkdir -p $(BUILD_DIR)
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) \
		-o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 ./cmd/server

build-darwin-amd64:
	mkdir -p $(BUILD_DIR)
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) \
		-o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 ./cmd/server

## build-darwin-webview: build the WKWebView binary (macOS + Xcode required, CGO enabled)
build-darwin-webview:
	mkdir -p $(BUILD_DIR)
	CGO_ENABLED=1 GOOS=darwin go build $(LDFLAGS) \
		-o $(BUILD_DIR)/$(BINARY_NAME)-darwin-webview ./cmd/app

## icon: generate AppIcon.icns from the SVG source (macOS only, requires: brew install librsvg)
icon:
	@bash scripts/make-icon.sh

## app: build the primary macOS .app bundle with a native WKWebView window.
##      Requires macOS + Xcode command-line tools (CGO_ENABLED=1).
##      Run 'make icon' first to include the app icon.
app: build-darwin-webview
	@echo "Building .app bundle → $(APP_BUNDLE)"
	@bash scripts/package-macos.sh \
		--binary   "$(BUILD_DIR)/$(BINARY_NAME)-darwin-webview" \
		--name     "$(APP_NAME)" \
		--version  "$(VERSION)" \
		--out-dir  "$(BUILD_DIR)" \
		--webview

## app-server: build a headless server .app bundle that opens the UI in the
##             default browser (cross-compilable, no CGO required).
##             Run 'make build-darwin' first when cross-compiling from Linux/Windows.
app-server: _require-darwin-binary
	@echo "Building headless server .app bundle → $(APP_BUNDLE)"
	@bash scripts/package-macos.sh \
		--binary   "$(BUILD_DIR)/$(BINARY_NAME)-darwin-universal" \
		--name     "$(APP_NAME)" \
		--version  "$(VERSION)" \
		--out-dir  "$(BUILD_DIR)"

## dmg: create a distributable .dmg (requires 'make app' first, macOS only)
dmg: _require-app-bundle
	@echo "Building .dmg → $(DMG_PATH)"
	@bash scripts/package-macos.sh \
		--binary   "$(BUILD_DIR)/$(BINARY_NAME)-darwin-webview" \
		--name     "$(APP_NAME)" \
		--version  "$(VERSION)" \
		--out-dir  "$(BUILD_DIR)" \
		--webview \
		--dmg

## clean: remove build artifacts
clean:
	rm -rf $(BUILD_DIR)

## test: run all tests
test:
	go test ./...

## lint: run go vet
lint:
	go vet ./...

# ── internal helpers ────────────────────────────────────────────────────────

_require-darwin-binary:
	@if [ ! -f "$(BUILD_DIR)/$(BINARY_NAME)-darwin-universal" ] && \
	    [ ! -f "$(BUILD_DIR)/$(BINARY_NAME)" ]; then \
		echo "No darwin binary found. Run 'make build-darwin' first."; \
		exit 1; \
	fi
	@# If only the plain binary exists (running on macOS natively), symlink it.
	@if [ ! -f "$(BUILD_DIR)/$(BINARY_NAME)-darwin-universal" ] && \
	    [ -f "$(BUILD_DIR)/$(BINARY_NAME)" ]; then \
		cp "$(BUILD_DIR)/$(BINARY_NAME)" \
		   "$(BUILD_DIR)/$(BINARY_NAME)-darwin-universal"; \
	fi

_require-app-bundle:
	@if [ ! -d "$(APP_BUNDLE)" ]; then \
		echo ".app bundle not found at $(APP_BUNDLE). Run 'make app' first."; \
		exit 1; \
	fi

help:
	@echo "Available targets:"
	@grep -E '^## ' Makefile | sed 's/## /  /'
