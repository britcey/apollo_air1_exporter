package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type sensorDefinition struct {
	id, name, metric, help, domain string
}

var sensors = []sensorDefinition{
	{"co2", "CO2", "co2_ppm", "CO2 concentration in parts per million", "sensor"},
	{"sen55_temperature", "SEN55 Temperature", "temperature_celsius", "Temperature in degrees Celsius", "sensor"},
	{"sen55_humidity", "SEN55 Humidity", "humidity_percent", "Relative humidity percentage", "sensor"},
	{"pm__1_m_weight_concentration", "PM <1\u00b5m Weight concentration", "pm1_0_ugm3", "PM1.0 particulate matter in micrograms per cubic meter", "sensor"},
	{"pm__2_5_m_weight_concentration", "PM <2.5\u00b5m Weight concentration", "pm2_5_ugm3", "PM2.5 particulate matter in micrograms per cubic meter", "sensor"},
	{"pm__4_m_weight_concentration", "PM <4\u00b5m Weight concentration", "pm4_0_ugm3", "PM4.0 particulate matter in micrograms per cubic meter", "sensor"},
	{"pm__10_m_weight_concentration", "PM <10\u00b5m Weight concentration", "pm10_0_ugm3", "PM10 particulate matter in micrograms per cubic meter", "sensor"},
	{"pm_0_3_to_1__m", "PM 0.3 To 1 \u00b5m", "pm_0_3_to_1_ugm3", "PM 0.3 to 1 particulate matter in micrograms per cubic meter", "sensor"},
	{"pm_1_to_2_5__m", "PM 1 To 2.5 \u00b5m", "pm_1_to_2_5_ugm3", "PM 1 to 2.5 particulate matter in micrograms per cubic meter", "sensor"},
	{"pm_2_5_to_4__m", "PM 2.5 To 4 \u00b5m", "pm_2_5_to_4_ugm3", "PM 2.5 to 4 particulate matter in micrograms per cubic meter", "sensor"},
	{"pm_4_to_10__m", "PM 4 To 10 \u00b5m", "pm_4_to_10_ugm3", "PM 4 to 10 particulate matter in micrograms per cubic meter", "sensor"},
	{"carbon_monoxide", "Carbon Monoxide", "carbon_monoxide_ppm", "Carbon Monoxide in parts per million", "sensor"},
	{"methane", "Methane", "methane_ppm", "Methane in parts per million", "sensor"},
	{"ethanol", "Ethanol", "ethanol_ppm", "Ethanol in parts per million", "sensor"},
	{"hydrogen", "Hydrogen", "hydrogen_ppm", "Hydrogen in parts per million", "sensor"},
	{"ammonia", "Ammonia", "ammonia_ppm", "Ammonia in parts per million", "sensor"},
	{"nitrogen_dioxide", "Nitrogen Dioxide", "nitrogen_dioxide_ppm", "Nitrogen Dioxide in parts per million", "sensor"},
	{"sen55_voc", "SEN55 VOC", "voc_index", "Volatile Organic Compounds index", "sensor"},
	{"sen55_nox", "SEN55 NOX", "nox_index", "Nitrogen Oxides index", "sensor"},
	{"dps310_pressure", "DPS310 Pressure", "pressure_hpa", "Atmospheric pressure in hectopascals", "sensor"},
	{"esp_temperature", "ESP Temperature", "esp_temperature_celsius", "ESP32 internal temperature in degrees Celsius", "sensor"},
	{"rssi", "RSSI", "wifi_rssi_dbm", "WiFi signal strength in dBm", "sensor"},
	{"uptime", "Uptime", "uptime_seconds", "Device uptime in seconds", "sensor"},
	{"nowcast_aqi", "NowCast AQI", "aqi", "NowCast Air Quality Index reported by the device", "sensor"},
	{"online", "Online", "online", "Device Online sensor (1 for on, 0 for off)", "binary_sensor"},
	{"voc_quality", "VOC Quality", "voc_quality", "VOC quality enum (1 for the current state, 0 for other states)", "text_sensor"},
	{"ip_address", "IP Address", "ip_address_info", "Device IP address (value always 1)", "text_sensor"},
	{"esphome_version", "ESPHome Version", "esphome_version_info", "ESPHome version (value always 1)", "text_sensor"},
	{"apollo_firmware_version", "Apollo Firmware Version", "apollo_firmware_version_info", "Apollo firmware version (value always 1)", "text_sensor"},
}

type apolloClient struct {
	httpClient *http.Client
	baseURL    string
}

func newApolloClient(host string, timeout time.Duration) *apolloClient {
	return &apolloClient{httpClient: &http.Client{Timeout: timeout}, baseURL: strings.TrimRight(host, "/")}
}

func (client *apolloClient) getSensor(ctx context.Context, name string) (float64, error) {
	value, err := client.getReading(ctx, "sensor", name)
	if err != nil {
		return 0, err
	}
	return value.(float64), nil
}

func (client *apolloClient) getReading(ctx context.Context, domain, name string) (any, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, client.baseURL+"/"+domain+"/"+url.QueryEscape(name), nil)
	if err != nil {
		return 0, err
	}
	response, err := client.httpClient.Do(request)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return 0, fmt.Errorf("sensor %s: HTTP %d", name, response.StatusCode)
	}
	var data struct {
		ID    *string `json:"id"`
		Value any     `json:"value"`
		State *string `json:"state"`
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 1<<20))
	if err := decoder.Decode(&data); err != nil {
		return 0, fmt.Errorf("sensor %s: %w", name, err)
	}
	if data.ID == nil || data.Value == nil || data.State == nil {
		return 0, fmt.Errorf("sensor %s: missing id, value or state", name)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return 0, fmt.Errorf("sensor %s: trailing JSON data", name)
	}
	switch domain {
	case "sensor":
		if _, ok := data.Value.(float64); ok {
			return data.Value, nil
		}
	case "text_sensor":
		if _, ok := data.Value.(string); ok {
			return data.Value, nil
		}
	case "binary_sensor":
		if _, ok := data.Value.(bool); ok {
			return data.Value, nil
		}
	}
	return nil, fmt.Errorf("sensor %s: invalid value for %s", name, domain)
}

func (client *apolloClient) getStatus(ctx context.Context) (map[string]any, error) {
	values := make(map[string]any)
	for _, sensor := range sensors {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		value, err := client.getReading(ctx, sensor.domain, sensor.name)
		if err != nil {
			slog.Debug("Sensor unavailable", "host", client.baseURL, "sensor", sensor.name, "error", err)
			continue
		}
		values[sensor.id] = value
	}
	if len(values) == 0 {
		return nil, fmt.Errorf("no sensors found on device")
	}
	return values, nil
}
