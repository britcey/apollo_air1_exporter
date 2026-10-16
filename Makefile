.PHONY: build build-linux build-musl crossbuild tarball promu test lint fmt fmt-check clean docker-build docker-buildx docker-push docker-push-ghcr run run-debug check dev-setup gh-secrets release release-patch release-minor release-major help

VERSION ?= $(shell sed -n '1p' VERSION)
GOARCH ?= $(shell go env GOARCH)
GO ?= go
FIRST_GOPATH := $(firstword $(subst :, ,$(shell $(GO) env GOPATH)))
PROMU ?= $(FIRST_GOPATH)/bin/promu
PROMU_VERSION ?= 0.20.0
GNU_COREUTILS_BIN := $(shell brew --prefix coreutils 2>/dev/null)/libexec/gnubin

# Build the project
build: promu
	$(PROMU) build

crossbuild: promu
	$(PROMU) crossbuild

tarball: build promu
	@if [ "$$(uname)" = Darwin ] && [ ! -x "$(GNU_COREUTILS_BIN)/cp" ]; then \
		echo "Promu tarball requires GNU coreutils on macOS (brew install coreutils)" >&2; \
		exit 1; \
	elif [ -x "$(GNU_COREUTILS_BIN)/cp" ]; then \
		PATH="$(GNU_COREUTILS_BIN):$$PATH" $(PROMU) tarball --prefix dist dist; \
	else \
		$(PROMU) tarball --prefix dist dist; \
	fi

promu: $(PROMU)

$(PROMU):
	GOBIN="$(FIRST_GOPATH)/bin" $(GO) install github.com/prometheus/promu@v$(PROMU_VERSION)

# Run tests
test:
	go test -race ./...

# Run linting
lint:
	go vet ./...

# Format code
fmt:
	gofmt -w *.go

fmt-check:
	@test -z "$$(gofmt -l *.go)" || { gofmt -l *.go; exit 1; }

# Clean build artifacts
clean:
	go clean
	rm -rf dist/ .build/ .tarballs/

# Static Linux binaries need no cross-linker or C toolchain.
build-linux: promu
	GOOS=linux GOARCH=$(GOARCH) $(PROMU) build --prefix dist/linux-$(GOARCH)

build-musl: build-linux

docker-build: build-linux
	docker build --target prebuilt -f Dockerfile -t apollo_air1_exporter dist/linux-$(GOARCH)
	docker run --rm apollo_air1_exporter --help >/dev/null

# Build multi-arch Docker image (local)
docker-buildx:
	docker buildx build --platform linux/amd64,linux/arm64 -t apollo_air1_exporter .

# Build and push multi-arch Docker image to Docker Hub
docker-push:
	@if [ -z "$$DOCKER_USERNAME" ]; then \
		echo "Error: DOCKER_USERNAME environment variable is required"; \
		echo "Usage: DOCKER_USERNAME=youruser DOCKER_PASSWORD=yourpass make docker-push"; \
		exit 1; \
	fi
	@if [ -z "$$DOCKER_PASSWORD" ]; then \
		echo "Error: DOCKER_PASSWORD environment variable is required"; \
		echo "Usage: DOCKER_USERNAME=youruser DOCKER_PASSWORD=yourpass make docker-push"; \
		exit 1; \
	fi
	@echo "Logging in to Docker Hub..."
	@echo "$$DOCKER_PASSWORD" | docker login -u "$$DOCKER_USERNAME" --password-stdin
	@echo "Building and pushing multi-arch images..."
	docker buildx build --platform linux/amd64,linux/arm64 \
		-t $$DOCKER_USERNAME/apollo_air1_exporter:latest \
		-t $$DOCKER_USERNAME/apollo_air1_exporter:$$(git describe --tags --always) \
		--push .
	@echo "Successfully pushed to Docker Hub!"

# Build and push to GitHub Container Registry
docker-push-ghcr:
	@if [ -z "$$GITHUB_TOKEN" ]; then \
		echo "Error: GITHUB_TOKEN environment variable is required"; \
		exit 1; \
	fi
	@echo "Logging in to GitHub Container Registry..."
	@echo "$$GITHUB_TOKEN" | docker login ghcr.io -u $$GITHUB_ACTOR --password-stdin
	@echo "Building and pushing multi-arch images to GHCR..."
	docker buildx build --platform linux/amd64,linux/arm64 \
		-t ghcr.io/$$GITHUB_REPOSITORY_OWNER/apollo_air1_exporter:latest \
		-t ghcr.io/$$GITHUB_REPOSITORY_OWNER/apollo_air1_exporter:$$(git describe --tags --always) \
		--push .
	@echo "Successfully pushed to GitHub Container Registry!"

# Run locally
run:
	go run .

# Run with debug logging
run-debug:
	APOLLO_LOG_LEVEL=debug \
	go run .

# Check code before committing
check: fmt-check lint test

# Prepare a release
release:
	@if [ -z "$$VERSION" ]; then \
		echo "Error: VERSION environment variable is required"; \
		echo "Usage: VERSION=v0.1.0 make release"; \
		exit 1; \
	fi
	@bash scripts/release.sh "$(VERSION)"

# Install development dependencies
dev-setup:
	go mod download

# Deploy secrets to GitHub
gh-secrets:
	@if [ ! -f .env ]; then \
		echo "Error: .env file not found. Copy .env.example to .env and fill in your values."; \
		exit 1; \
	fi
	@echo "Loading environment variables from .env..."
	@export $$(grep -v '^#' .env | xargs) && \
		gh secret set DOCKER_USERNAME --body "$$DOCKER_USERNAME" && \
		echo "✓ Set DOCKER_USERNAME" && \
		gh secret set DOCKER_PASSWORD --body "$$DOCKER_PASSWORD" && \
		echo "✓ Set DOCKER_PASSWORD"
	@echo "GitHub secrets deployed successfully!"

# Show available commands
help:
	@echo "Available commands:"
	@echo "  make build          - Build the project with promu"
	@echo "  make crossbuild     - Cross-build configured platforms with promu"
	@echo "  make tarball        - Build a promu release tarball"
	@echo "  make test           - Run tests"
	@echo "  make lint           - Run linting"
	@echo "  make fmt            - Format code"
	@echo "  make clean          - Clean build artifacts"
	@echo "  make docker-build   - Build Docker image (local)"
	@echo "  make docker-buildx  - Build multi-arch Docker image (local)"
	@echo "  make docker-push    - Build and push multi-arch to Docker Hub"
	@echo "  make docker-push-ghcr - Build and push multi-arch to GitHub Container Registry"
	@echo "  make run            - Run locally"
	@echo "  make run-debug      - Run with debug logging"
	@echo "  make check          - Check formatting, lint, and test"
	@echo "  make release        - Prepare and create a release (requires VERSION=v0.1.0)"
	@echo "  make dev-setup      - Install development dependencies"
	@echo "  make gh-secrets     - Deploy secrets to GitHub"

release-patch release-minor release-major:
	@next=$$(printf '%s\n' '$(VERSION)' | sed 's/^v//' | \
		awk -F. -v part='$(@:release-%=%)' '{ \
			major=$$1; minor=$$2; patch=$$3; \
			if (part == "major") { major++; minor=0; patch=0 } \
			else if (part == "minor") { minor++; patch=0 } \
			else { patch++ } \
			printf "%d.%d.%d", major, minor, patch \
		}'); \
		$(MAKE) release VERSION="v$$next"
