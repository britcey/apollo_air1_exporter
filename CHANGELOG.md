# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## 0.4.3 / 2026-10-16

Release-related fixes

## 0.4.2 / 2026-10-16

Cross-target build fixes

## v0.4.1

Various build-related changes

## [Unreleased]

### Fixed

- Fetch sensors by their URL-encoded ESPHome display names instead of short IDs.

### Changed

- Ported [the exporter](https://github.com/rvben/apollo-air1-exporter) from Rust to Go.
- Switched builds, tests, Docker images, CI, and releases to the Go toolchain;
  releases no longer publish a Rust crate.
- Collect synchronously on `/metrics?target=http://device` using isolated
  per-scrape registries and request cancellation; configure friendly labels in
  Prometheus rather than through an exporter `name` parameter.
- Remove background polling, cached readings, startup device lists, and the
  `--hosts`, `--names`, and `--poll-interval` configuration options.
- Export all 29 device sensor readings, including device-provided NowCast AQI,
  text Info gauges, and a six-state VOC Quality enum instead of calculating AQI.
- Validate configuration and shut down gracefully on SIGINT or SIGTERM.
- Logging now uses APOLLO_LOG_LEVEL rather than Rust-specific RUST_LOG filters.

