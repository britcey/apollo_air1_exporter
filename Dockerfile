FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS build
ARG TARGETOS
ARG TARGETARCH
ARG VERSION
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY VERSION ./VERSION
COPY *.go ./
RUN VERSION="${VERSION:-$(cat VERSION)}" \
    BUILD_DATE="$(date -u +%Y%m%d-%H:%M:%S)" && \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath \
    -ldflags "-s -w -X github.com/prometheus/common/version.Version=$VERSION -X github.com/prometheus/common/version.Revision=unknown -X github.com/prometheus/common/version.Branch=unknown -X github.com/prometheus/common/version.BuildUser=unknown@unknown -X github.com/prometheus/common/version.BuildDate=$BUILD_DATE" \
    -o /out/apollo_air1_exporter .

FROM alpine:3.24 AS runtime

# OCI labels for GitHub Container Registry
LABEL org.opencontainers.image.source=https://github.com/britcey/apollo_air1_exporter
LABEL org.opencontainers.image.description="Prometheus exporter for Apollo AIR-1 air quality monitors"
LABEL org.opencontainers.image.licenses=MIT

# Install runtime dependencies
RUN apk add --no-cache ca-certificates

# Create non-root user
RUN addgroup -g 1000 exporter && \
    adduser -D -u 1000 -G exporter exporter

# Switch to non-root user
USER exporter

# Expose metrics port
EXPOSE 9926

# Set default environment variables
ENV APOLLO_LOG_LEVEL=info
ENV APOLLO_EXPORTER_PORT=9926

# Run the exporter
ENTRYPOINT ["/usr/local/bin/apollo_air1_exporter"]

FROM runtime AS prebuilt
COPY --chmod=755 apollo_air1_exporter /usr/local/bin/apollo_air1_exporter

FROM runtime AS final
COPY --from=build --chmod=755 /out/apollo_air1_exporter /usr/local/bin/apollo_air1_exporter
