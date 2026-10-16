# Apollo Air-1 Prometheus Exporter

[![CI](https://github.com/britcey/apollo_air1_exporter/actions/workflows/ci.yml/badge.svg)](https://github.com/britcey/apollo_air1_exporter/actions/workflows/ci.yml)
[![Docker Pulls](https://img.shields.io/docker/pulls/britcey/apollo_air1_exporter)](https://hub.docker.com/r/britcey/apollo_air1_exporter)
[![Docker Image Size](https://img.shields.io/docker/image-size/britcey/apollo_air1_exporter)](https://hub.docker.com/r/britcey/apollo_air1_exporter)
[![License](https://img.shields.io/github/license/britcey/apollo_air1_exporter)](LICENSE)

A Prometheus exporter for Apollo Air-1 air quality monitors (ESPHome-based devices).

Ported from [apollo-air1-exporter](https://github.com/rvben/apollo-air1-exporter) - big thanks to @rvben for the work.
  - Moved from Rust to Go (no real Rust experience)
  - Fetch sensors using full names - short names are not working with my AIR-1 - e.g., /sensor/Nitrogen+Dioxide 
  - Moved to a more "Promethean" design of fetching synchronously and specifying the target at scrape-time.
    - This matches other proxy-exporter patterns, like snmp_exporter

## Features

- Exports air quality metrics from Apollo Air-1 devices
- Supports multiple devices through scrape-time target parameters

## Metrics

The exporter exposes all 29 sensor readings in `events.json` (24 numeric sensors,
four text sensors, and the Online binary sensor), when available on the device,
plus exporter-observed device availability:

- `apollo_air1_device_up` - Device availability (1 = up, 0 = down)
- `apollo_air1_co2_ppm` - CO2 concentration in parts per million
- `apollo_air1_pm1_0_ugm3` - PM1.0 particulate matter in µg/m³
- `apollo_air1_pm2_5_ugm3` - PM2.5 particulate matter in µg/m³
- `apollo_air1_pm4_0_ugm3` - PM4.0 particulate matter in µg/m³
- `apollo_air1_pm10_0_ugm3` - PM10 particulate matter in µg/m³
- `apollo_air1_pm_0_3_to_1_ugm3` - PM0.3–1 particulate matter in µg/m³
- `apollo_air1_pm_1_to_2_5_ugm3` - PM1–2.5 particulate matter in µg/m³
- `apollo_air1_pm_2_5_to_4_ugm3` - PM2.5–4 particulate matter in µg/m³
- `apollo_air1_pm_4_to_10_ugm3` - PM4–10 particulate matter in µg/m³
- `apollo_air1_carbon_monoxide_ppm` - Carbon monoxide concentration in parts per million
- `apollo_air1_methane_ppm` - Methane concentration in parts per million
- `apollo_air1_ethanol_ppm` - Ethanol concentration in parts per million
- `apollo_air1_hydrogen_ppm` - Hydrogen concentration in parts per million
- `apollo_air1_ammonia_ppm` - Ammonia concentration in parts per million
- `apollo_air1_nitrogen_dioxide_ppm` - Nitrogen dioxide concentration in parts per million
- `apollo_air1_voc_index` - Volatile Organic Compounds index
- `apollo_air1_nox_index` - Nitrogen Oxides index
- `apollo_air1_temperature_celsius` - Temperature in degrees Celsius
- `apollo_air1_humidity_percent` - Relative humidity percentage
- `apollo_air1_pressure_hpa` - Atmospheric pressure in hectopascals
- `apollo_air1_esp_temperature_celsius` - ESP32 internal temperature
- `apollo_air1_wifi_rssi_dbm` - WiFi signal strength in dBm
- `apollo_air1_uptime_seconds` - Device uptime in seconds
- `apollo_air1_aqi` - NowCast AQI reported by the AIR-1, without exporter-side calculation
- `apollo_air1_online` - Device Online binary sensor (1 = on, 0 = off)
- `apollo_air1_voc_quality` - VOC Quality enum, with a `state` label
- `apollo_air1_ip_address_info` - IP Address in the `value` label (gauge value always 1)
- `apollo_air1_esphome_version_info` - ESPHome Version in the `value` label (gauge value always 1)
- `apollo_air1_apollo_firmware_version_info` - Apollo Firmware Version in the `value` label (gauge value always 1)

All metrics include `device` and `host` labels for identification.
Sensor metrics also include a `sensor` label containing the ESPHome display name
(for example, `sensor="Carbon Monoxide"` or `sensor="NowCast AQI"`). Only the
exporter-observed `device_up` metric omits this label.

Sensors are fetched using their ESPHome display names, URL-encoded under
`/sensor/`, `/text_sensor/`, or `/binary_sensor/` according to their domain
(for example, `/sensor/Carbon+Monoxide`), not short object IDs.

VOC Quality exports six series with `state` values `Improved`, `Normal`,
`Abnormal`, `Very abnormal`, `Extremely abnormal`, and `Unknown`. The current
state is 1 and all others are 0; unrecognized text maps to `Unknown`.
Info gauges replace the previous `value` label when the reading changes.
The exporter no longer calculates AQI or exports `aqi_pm25`, `aqi_pm10`, or
`aqi_info`. Illuminance is not present in this device's sensor inventory.

## Configuration

The exporter is configured via environment variables:

- `APOLLO_EXPORTER_PORT` (default: 9926) - Port to expose metrics on
- `APOLLO_EXPORTER_BIND` (default: 0.0.0.0) - Bind address for metrics server
- `APOLLO_HTTP_TIMEOUT` (default: 10) - Total device collection timeout per scrape in seconds
- `APOLLO_LOG_LEVEL` (default: info) - Log level (trace, debug, info, warn, error)

Equivalent command-line flags are `--port` (`-p`), `--bind`, `--http-timeout`,
and `--log-level`. Flags override environment variables. Timeouts are positive integer
seconds. Use `--help` for options and `--version` for the binary version.

Each request to `/probe` must supply one `target` query parameter containing
an `http://` or `https://` device URL. The `device` label is the target hostname.
Friendly labels can be configured in Prometheus. Targets may not contain
credentials, query parameters, or fragments. Missing or invalid targets return HTTP 400.

Collection happens synchronously for that target only, with a fresh registry
per request. There is no background polling, shared cache, or startup device list.
Prometheus controls the scrape interval. Failed collection returns HTTP 200 with
`device_up=0` and no stale readings; unavailable individual sensors are omitted.
The collection timeout bounds the whole scrape's device requests, and cancelling
the incoming request cancels collection. Set Prometheus's `scrape_timeout` above
`APOLLO_HTTP_TIMEOUT`. `/health` reports exporter liveness without querying devices.
The old `--hosts`, `--names`, and `--poll-interval` flags and corresponding
environment variables are no longer used.

Keep the exporter on a trusted monitoring network: scrape callers choose the
device URL the exporter contacts. Logging uses `APOLLO_LOG_LEVEL`;
the Rust-specific `RUST_LOG` filter is no longer supported.

## Installation

### Docker (Recommended)

Pull the pre-built multi-architecture image from Docker Hub:

```bash
docker pull britcey/apollo_air1_exporter:latest
```

Run with Docker:

```bash
docker run -d \
  --name apollo_air1_exporter \
  -p 9926:9926 \
  britcey/apollo_air1_exporter:latest
```

### Building from source

Requires Go 1.27 or newer.

```bash
git clone https://github.com/britcey/apollo_air1_exporter.git
cd apollo_air1_exporter
make build
```

### Running locally

```bash
./dist/apollo_air1_exporter
```

Query a device from another terminal:

```bash
curl --get 'http://localhost:9926/probe' \
  --data-urlencode 'target=http://192.168.1.100'
```

### Docker Compose

```yaml
version: '3'
services:
  apollo_air1_exporter:
    build: .
    ports:
      - "9926:9926"
    environment:
      - APOLLO_LOG_LEVEL=info
    restart: unless-stopped
```

## Endpoints

- `/probe?target=http://device` - Collect fresh metrics from one device
- `/metrics` - Exporter build, Go runtime, and process metrics
- `/health` - Health check endpoint
- `/` - Welcome page

## Building from source

```bash
make build
```

`make check` runs formatting checks, `go vet`, and race-enabled tests.
`make build-linux GOARCH=amd64` (or `arm64`) builds a static Linux binary without
a cross-compiler. `docker build -t apollo_air1_exporter .` and Docker Compose build
from source; CI can use the `prebuilt` Docker target with a binary-only context.

## Prometheus Configuration

Add the following to your `prometheus.yml`:

```yaml
scrape_configs:
  - job_name: 'apollo_air1'
    metrics_path: /probe
    scrape_interval: 30s
    scrape_timeout: 15s
    static_configs:
      - targets:
          - http://192.168.1.100
        labels:
          location: 'Living Room'
      - targets:
          - http://192.168.1.101
        labels:
          location: 'Bedroom'
    relabel_configs:
      - source_labels: [__address__]
        target_label: __param_target
      - source_labels: [__param_target]
        target_label: instance
      - target_label: __address__
        replacement: localhost:9926
```

Replace `localhost:9926` with the exporter address reachable from Prometheus.
The device URLs become the `target` parameter and `instance` label; each device
is collected independently by the same exporter.
Prometheus attaches the configured `location` label to every sample from that
target. Use a distinct label such as `location` to avoid colliding with the
exporter's hostname-based `device` label.

## License

Same as the other exporters in this repository.
