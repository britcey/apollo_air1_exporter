FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS build
ARG TARGETOS
ARG TARGETARCH
ARG VERSION
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath \
    -ldflags "-s -w ${VERSION:+-X main.version=$VERSION}" -o /out/apollo-air1-exporter .

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
ENTRYPOINT ["/usr/local/bin/apollo-air1-exporter"]

FROM runtime AS prebuilt
COPY --chmod=755 apollo-air1-exporter /usr/local/bin/apollo-air1-exporter

FROM runtime AS final
COPY --from=build --chmod=755 /out/apollo-air1-exporter /usr/local/bin/apollo-air1-exporter
