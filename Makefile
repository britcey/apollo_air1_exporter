.PHONY: build build-linux build-musl test lint fmt fmt-check clean docker-build docker-buildx docker-push docker-push-ghcr run run-debug check dev-setup gh-secrets release release-patch release-minor release-major help

VERSION ?= $(shell sed -n 's/^var version = "\(.*\)"/\1/p' main.go)
GOARCH ?= $(shell go env GOARCH)
LDFLAGS = -s -w -X main.version=$(VERSION)

# Build the project
build:
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o dist/apollo-air1-exporter .

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
	rm -rf dist/

# Static Linux binaries need no cross-linker or C toolchain.
build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=$(GOARCH) go build -trimpath -ldflags "$(LDFLAGS)" -o dist/linux-$(GOARCH)/apollo-air1-exporter .

build-musl: build-linux

docker-build: build-linux
	docker build --target prebuilt -f Dockerfile -t apollo-air1-exporter dist/linux-$(GOARCH)
	docker run --rm apollo-air1-exporter --help >/dev/null

# Build multi-arch Docker image (local)
docker-buildx:
	docker buildx build --platform linux/amd64,linux/arm64 -t apollo-air1-exporter .

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
		-t $$DOCKER_USERNAME/apollo-air1-exporter:latest \
		-t $$DOCKER_USERNAME/apollo-air1-exporter:$$(git describe --tags --always) \
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
		-t ghcr.io/$$GITHUB_REPOSITORY_OWNER/apollo-air1-exporter:latest \
		-t ghcr.io/$$GITHUB_REPOSITORY_OWNER/apollo-air1-exporter:$$(git describe --tags --always) \
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
	@echo "  make build          - Build the project"
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
