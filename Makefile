# Makefile for cleanup-tool

BINARY_NAME := cleanup-tool
CMD_PATH := ./cmd/cleanup-tool
BUILD_DIR := .
DIST_DIR := dist
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

# Release artifact names
DARWIN_ARM64 := $(BINARY_NAME)-darwin-arm64
DARWIN_AMD64 := $(BINARY_NAME)-darwin-amd64
DARWIN_UNIVERSAL := $(BINARY_NAME)-darwin-universal
TARBALL := $(BINARY_NAME)-$(VERSION)-darwin-universal.tar.gz

# Allow extra linker flags to be passed in (e.g., for version embedding)
LDFLAGS ?=

# Default installation directory for `make install`
INSTALL_DIR ?= $(HOME)/bin

# GPG configuration for release signing
GPG ?= gpg
GPG_KEY_ID ?=
# Set GPG_PASSPHRASE when running in a non-interactive environment
# to use loopback pinentry (requires `allow-loopback-pinentry` in gpg-agent.conf).
GPG_PASSPHRASE ?=

GPG_ARGS := --batch --armor --detach-sign --yes
ifneq ($(GPG_KEY_ID),)
	GPG_ARGS += -u $(GPG_KEY_ID)
endif
ifneq ($(GPG_PASSPHRASE),)
	GPG_ARGS += --pinentry-mode loopback --passphrase "$(GPG_PASSPHRASE)"
endif

.DEFAULT_GOAL := build

.PHONY: build test vet clean run bench help install release release-arm64 release-amd64 release-universal release-tarball release-checksums release-sign release-clean

build: ## Build the cleanup-tool binary for the current host
	go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME) $(CMD_PATH)

test: ## Run all Go tests
	go test ./...

vet: ## Run go vet on all packages
	go vet ./...

clean: ## Remove the built binary
	rm -f $(BUILD_DIR)/$(BINARY_NAME)

run: build ## Build and run the TUI
	$(BUILD_DIR)/$(BINARY_NAME)

bench: build ## Build and run a quick benchmark scan
	$(BUILD_DIR)/$(BINARY_NAME) -benchmark -paths /tmp

release: release-universal release-tarball release-checksums ## Build universal binary, tarball, and checksums

release-arm64: ## Build macOS arm64 binary
	@mkdir -p $(DIST_DIR)
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(DARWIN_ARM64) $(CMD_PATH)

release-amd64: ## Build macOS amd64 binary
	@mkdir -p $(DIST_DIR)
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o $(DIST_DIR)/$(DARWIN_AMD64) $(CMD_PATH)

release-universal: release-arm64 release-amd64 ## Build a macOS universal binary (requires macOS lipo)
	@mkdir -p $(DIST_DIR)
	@if ! command -v lipo >/dev/null 2>&1; then \
		echo "Error: lipo is required to build the universal binary and is only available on macOS." >&2; \
		exit 1; \
	fi
	lipo -create -output $(DIST_DIR)/$(DARWIN_UNIVERSAL) $(DIST_DIR)/$(DARWIN_ARM64) $(DIST_DIR)/$(DARWIN_AMD64)

release-tarball: release-universal ## Create a gzipped tarball of the universal binary and README
	@mkdir -p $(DIST_DIR)/tarball/$(BINARY_NAME)-$(VERSION)
	cp $(DIST_DIR)/$(DARWIN_UNIVERSAL) $(DIST_DIR)/tarball/$(BINARY_NAME)-$(VERSION)/$(BINARY_NAME)
	cp README.md $(DIST_DIR)/tarball/$(BINARY_NAME)-$(VERSION)/
	-cp CHANGELOG.md $(DIST_DIR)/tarball/$(BINARY_NAME)-$(VERSION)/ 2>/dev/null || true
	tar -czf $(DIST_DIR)/$(TARBALL) -C $(DIST_DIR)/tarball $(BINARY_NAME)-$(VERSION)
	@rm -rf $(DIST_DIR)/tarball

release-checksums: ## Generate SHA-256 checksums for release tarballs
	@cd $(DIST_DIR) && shasum -a 256 *.tar.gz > checksums.txt

install: release-universal ## Install the universal binary to ~/bin (override with INSTALL_DIR=)
	@mkdir -p $(INSTALL_DIR)
	install -m 0755 $(DIST_DIR)/$(DARWIN_UNIVERSAL) $(INSTALL_DIR)/$(BINARY_NAME)
	@echo "Installed $(BINARY_NAME) to $(INSTALL_DIR)"

release-sign: ## Sign release tarballs and checksums with GPG
	@if ! command -v $(GPG) >/dev/null 2>&1; then \
		echo "Error: $(GPG) not found. Please install GnuPG to sign releases." >&2; \
		exit 1; \
	fi
	@for f in $(DIST_DIR)/*.tar.gz; do \
		if [ -f "$$f" ]; then \
			echo "Signing $$f..."; \
			$(GPG) $(GPG_ARGS) "$$f"; \
		fi; \
	done
	@if [ -f "$(DIST_DIR)/checksums.txt" ]; then \
		echo "Signing $(DIST_DIR)/checksums.txt..."; \
		$(GPG) $(GPG_ARGS) "$(DIST_DIR)/checksums.txt"; \
	fi

release-clean: ## Remove all release artifacts
	rm -rf $(DIST_DIR)

release-clean-old: ## Remove all but the most recent release tarball in dist/
	@cd $(DIST_DIR) && { \
		tarballs=$$(ls -t *.tar.gz 2>/dev/null); \
		if [ -z "$$tarballs" ]; then \
			echo "No tarballs found in $(DIST_DIR)."; \
			exit 0; \
		fi; \
		kept=$$(echo "$$tarballs" | head -n 1); \
		echo "Keeping: $$kept"; \
		echo "$$tarballs" | tail -n +2 | while read -r old; do \
			echo "Removing: $$old"; \
			rm -f "$$old"; \
		done; \
	}

watch: ## Watch Go files and rebuild on change (requires reflex)
	@if ! command -v reflex >/dev/null 2>&1; then \
		echo "reflex is not installed. Install it with:" >&2; \
		echo "  go install github.com/cespare/reflex@latest" >&2; \
		exit 1; \
	fi
	reflex -r '\.go$$' -s -- sh -c 'make build'

watch-test: ## Watch Go files and run tests on change (requires reflex)
	@if ! command -v reflex >/dev/null 2>&1; then \
		echo "reflex is not installed. Install it with:" >&2; \
		echo "  go install github.com/cespare/reflex@latest" >&2; \
		exit 1; \
	fi
	reflex -r '\.go$$' -s -- sh -c 'make test'

help: ## Show this help message
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'
