package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
)

func TestConfig(t *testing.T) {
	env := map[string]string{
		"APOLLO_EXPORTER_PORT": "1234", "APOLLO_HTTP_TIMEOUT": "15",
	}
	getenv := func(key string) string { return env[key] }
	cfg, err := parseConfig([]string{"--port", "9926", "--bind", "::1"}, getenv, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.address() != "[::1]:9926" || cfg.httpTimeout != 15*time.Second {
		t.Fatalf("unexpected configuration: %+v", cfg)
	}
	defaults, err := parseConfig(nil, func(string) string { return "" }, io.Discard)
	if err != nil || defaults.address() != "0.0.0.0:9926" || defaults.httpTimeout != 10*time.Second || defaults.logLevel != "info" {
		t.Fatalf("invalid defaults: %+v, %v", defaults, err)
	}
	for _, args := range [][]string{
		{"--hosts=http://apollo"}, {"--names=Room"}, {"--poll-interval=30"},
		{"--http-timeout=0"}, {"--http-timeout=-1"}, {"--http-timeout=9223372036854775807"},
		{"--port=65536"}, {"--log-level=nope"}, {"unexpected"},
	} {
		if _, err := parseConfig(args, func(string) string { return "" }, io.Discard); err == nil {
			t.Errorf("expected error for %v", args)
		}
	}
	for _, args := range [][]string{{"--help"}, {"-h"}, {"--version"}, {"-V"}} {
		var output bytes.Buffer
		if _, err := parseConfig(args, getenv, &output); !errors.Is(err, flag.ErrHelp) || output.Len() == 0 {
			t.Errorf("%v: output=%q, error=%v", args, output.String(), err)
		}
	}
}

func TestEventSensorCoverage(t *testing.T) {
	file, err := os.Open("events.json")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	responses := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		body, ok := strings.CutPrefix(scanner.Text(), "data: ")
		if !ok || !json.Valid([]byte(body)) {
			continue
		}
		var event struct {
			Domain string `json:"domain"`
			Name   string `json:"name"`
		}
		if err := json.Unmarshal([]byte(body), &event); err != nil {
			t.Fatal(err)
		}
		if event.Domain == "sensor" || event.Domain == "text_sensor" || event.Domain == "binary_sensor" {
			responses["/"+event.Domain+"/"+url.QueryEscape(event.Name)] = body
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if len(responses) != 29 {
		t.Fatalf("fixture has %d sensors, want 29", len(responses))
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, ok := responses[request.RequestURI]
		if !ok {
			http.NotFound(writer, request)
			return
		}
		fmt.Fprint(writer, body)
	}))
	defer server.Close()
	values, err := newApolloClient(server.URL, time.Second).getStatus(context.Background())
	if err != nil || len(values) != 29 {
		t.Fatalf("got %d sensor readings, want 29: %v", len(values), err)
	}
	metrics := newMetrics()
	metrics.updateDevice(device{server.URL, "Fixture"}, values)
	families, err := metrics.registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	if len(sensors) != 29 || len(families) != 30 {
		t.Fatalf("got %d definitions and %d families, want 29 sensors plus device_up", len(sensors), len(families))
	}
	for _, sensor := range sensors {
		var event struct {
			Value any `json:"value"`
		}
		body := responses["/"+sensor.domain+"/"+url.QueryEscape(sensor.name)]
		if err := json.Unmarshal([]byte(body), &event); err != nil {
			t.Fatalf("sensor %s: %v", sensor.name, err)
		}
		if values[sensor.id] != event.Value {
			t.Errorf("%s = %v, want %v", sensor.name, values[sensor.id], event.Value)
		}
		for _, family := range families {
			if family.GetName() != "apollo_air1_"+sensor.metric {
				continue
			}
			for _, metric := range family.Metric {
				want := 1.0
				if number, ok := event.Value.(float64); ok {
					want = number
				}
				for _, label := range metric.Label {
					if sensor.id == "voc_quality" && label.GetName() == "state" && label.GetValue() != event.Value {
						want = 0
					}
					if label.GetName() == "value" && label.GetValue() != event.Value {
						t.Errorf("%s info label = %q, want %v", sensor.name, label.GetValue(), event.Value)
					}
				}
				if got := metric.GetGauge().GetValue(); got != want {
					t.Errorf("%s = %v, want %v", sensor.name, got, want)
				}
			}
		}
	}
}

func TestGetSensor(t *testing.T) {
	for _, test := range []struct {
		name, body string
		code       int
		valid      bool
	}{
		{"valid", `{"id":"sensor-co2","value":450,"state":"450 ppm"}`, 200, true},
		{"zero", `{"id":"sensor-co2","value":0,"state":"0 ppm"}`, 200, true},
		{"missing", `{"id":"sensor-co2","state":"450 ppm"}`, 200, false},
		{"null", `{"id":"sensor-co2","value":null,"state":"unknown"}`, 200, false},
		{"invalid", `{`, 200, false},
		{"trailing", `{"id":"co2","value":1,"state":"1"} {}`, 200, false},
		{"not found", "", 404, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.URL.Path != "/sensor/CO2" {
					t.Errorf("unexpected path: %s", request.URL.Path)
				}
				writer.WriteHeader(test.code)
				fmt.Fprint(writer, test.body)
			}))
			defer server.Close()
			_, err := newApolloClient(server.URL+"/", time.Second).getSensor(context.Background(), "CO2")
			if (err == nil) != test.valid {
				t.Errorf("error = %v, valid = %v", err, test.valid)
			}
		})
	}
}

func TestGetReadingTypes(t *testing.T) {
	for _, test := range []struct {
		name, domain, value string
		want                any
		valid               bool
	}{
		{"text", "text_sensor", `"Normal"`, "Normal", true},
		{"empty text", "text_sensor", `""`, "", true},
		{"binary on", "binary_sensor", `true`, true, true},
		{"binary off", "binary_sensor", `false`, false, true},
		{"numeric text", "text_sensor", `42`, nil, false},
		{"boolean text", "text_sensor", `true`, nil, false},
		{"null text", "text_sensor", `null`, nil, false},
		{"string binary", "binary_sensor", `"ON"`, nil, false},
		{"numeric binary", "binary_sensor", `1`, nil, false},
		{"null binary", "binary_sensor", `null`, nil, false},
		{"string numeric", "sensor", `"42"`, nil, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.RequestURI != "/"+test.domain+"/Test+Reading" {
					t.Errorf("unexpected path: %s", request.RequestURI)
				}
				fmt.Fprintf(writer, `{"id":"example","value":%s,"state":"example"}`, test.value)
			}))
			defer server.Close()
			value, err := newApolloClient(server.URL, time.Second).getReading(context.Background(), test.domain, "Test Reading")
			if (err == nil) != test.valid || (test.valid && value != test.want) {
				t.Fatalf("value=%v, error=%v; want %v, valid=%v", value, err, test.want, test.valid)
			}
		})
	}
}

func TestSensorDisplayNameURLs(t *testing.T) {
	for _, test := range []struct {
		id, path string
	}{
		{"co2", "/sensor/CO2"},
		{"carbon_monoxide", "/sensor/Carbon+Monoxide"},
		{"sen55_nox", "/sensor/SEN55+NOX"},
		{"pm__2_5_m_weight_concentration", "/sensor/PM+%3C2.5%C2%B5m+Weight+concentration"},
		{"pm__10_m_weight_concentration", "/sensor/PM+%3C10%C2%B5m+Weight+concentration"},
		{"pm_0_3_to_1__m", "/sensor/PM+0.3+To+1+%C2%B5m"},
		{"pm__4_m_weight_concentration", "/sensor/PM+%3C4%C2%B5m+Weight+concentration"},
		{"nowcast_aqi", "/sensor/NowCast+AQI"},
	} {
		t.Run(test.id, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.RequestURI != test.path {
					http.NotFound(writer, request)
					return
				}
				fmt.Fprint(writer, `{"id":"sensor/example","value":1.5,"state":"1.5"}`)
			}))
			defer server.Close()
			values, err := newApolloClient(server.URL, time.Second).getStatus(context.Background())
			if err != nil || len(values) != 1 || values[test.id] != 1.5 {
				t.Fatalf("expected %s to use %s: values=%v, error=%v", test.id, test.path, values, err)
			}
		})
	}
}

func TestClientTimeoutAndCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		<-request.Context().Done()
	}))
	defer server.Close()
	client := newApolloClient(server.URL, 20*time.Millisecond)
	if _, err := client.getSensor(context.Background(), "CO2"); err == nil {
		t.Fatal("expected HTTP timeout")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.getStatus(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
}

func TestScrapeTargetValidation(t *testing.T) {
	handler := newExporter(config{httpTimeout: time.Second}).handler()
	for _, target := range []string{"", "apollo.local", "http://", "ftp://apollo", "http://user:pass@apollo", "http://apollo?query=1", "http://apollo#fragment", "http://apollo:bad"} {
		t.Run(target, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest("GET", "/probe?target="+url.QueryEscape(target), nil))
			if response.Code != http.StatusBadRequest {
				t.Fatalf("invalid target returned %d: %s", response.Code, response.Body.String())
			}
		})
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest("GET", "/probe?target=http://one&target=http://two", nil))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("multiple targets returned %d", response.Code)
	}
}

func TestScrapeTimeoutAndCancellation(t *testing.T) {
	var calls atomic.Int32
	started := make(chan struct{}, 2)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		started <- struct{}{}
		<-request.Context().Done()
	}))
	defer server.Close()
	for _, cancelRequest := range []bool{false, true} {
		t.Run(fmt.Sprintf("cancel=%t", cancelRequest), func(t *testing.T) {
			timeout := 50 * time.Millisecond
			if cancelRequest {
				timeout = 5 * time.Second
			}
			handler := newExporter(config{httpTimeout: timeout}).handler()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			request := httptest.NewRequest("GET", "/probe?target="+url.QueryEscape(server.URL), nil).WithContext(ctx)
			response := httptest.NewRecorder()
			done := make(chan struct{})
			go func() {
				defer close(done)
				handler.ServeHTTP(response, request)
			}()
			select {
			case <-started:
			case <-time.After(time.Second):
				t.Fatal("scrape did not start a device request")
			}
			if cancelRequest {
				cancel()
			}
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("scrape did not stop after timeout or cancellation")
			}
			if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "} 0\n") || strings.Contains(response.Body.String(), "apollo_air1_co2_ppm") {
				t.Fatalf("failed scrape should only report device down: %d, %s", response.Code, response.Body.String())
			}
		})
	}
	if calls.Load() != 2 {
		t.Fatalf("collection continued after cancellation: %d device requests", calls.Load())
	}
}

func TestScrapeTargetAndFreshness(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		if request.URL.Path != "/sensor/CO2" {
			http.NotFound(writer, request)
			return
		}
		fmt.Fprintf(writer, `{"id":"co2","value":%d,"state":"ppm"}`, calls.Load())
	}))
	defer server.Close()
	handler := newExporter(config{httpTimeout: time.Second}).handler()
	if calls.Load() != 0 {
		t.Fatal("exporter contacted a device before a scrape")
	}
	for _, path := range []string{"/health", "/", "/metrics", "/probe"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest("GET", path, nil))
		want := http.StatusOK
		if path == "/probe" {
			want = http.StatusBadRequest
		}
		if response.Code != want || calls.Load() != 0 {
			t.Fatalf("%s: status=%d, device requests=%d", path, response.Code, calls.Load())
		}
		if path == "/metrics" && (!strings.Contains(response.Body.String(), `apollo_air1_exporter_info{version="`+version+`"} 1`) || !strings.Contains(response.Body.String(), "go_goroutines")) {
			t.Fatalf("/metrics is missing exporter info or Go runtime metrics: %s", response.Body.String())
		}
	}
	for range 2 {
		before := calls.Load()
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest("GET", "/probe?target="+url.QueryEscape(server.URL)+"&name=Living+Room", nil))
		want := fmt.Sprintf(`apollo_air1_co2_ppm{device="127.0.0.1",host=%q,sensor="CO2"} %d`, server.URL, before+1)
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), want) || calls.Load() != before+int32(len(sensors)) {
			t.Fatalf("scrape did not query fresh readings: status=%d, calls=%d, body=%s", response.Code, calls.Load(), response.Body.String())
		}
	}
}

func TestExporterScrapingAndRecovery(t *testing.T) {
	var online atomic.Bool
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		if !online.Load() {
			writer.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		domain, encodedName, _ := strings.Cut(strings.TrimPrefix(request.URL.EscapedPath(), "/"), "/")
		name, err := url.QueryUnescape(encodedName)
		if err != nil {
			t.Error(err)
			http.Error(writer, "invalid sensor name", http.StatusBadRequest)
			return
		}
		var id string
		for _, sensor := range sensors {
			if sensor.name == name && sensor.domain == domain {
				id = sensor.id
				break
			}
		}
		if id == "" {
			http.NotFound(writer, request)
			return
		}
		var value any = 12.5
		if id == "rssi" {
			value = -62.9
		}
		if domain == "text_sensor" {
			value = "Normal"
		}
		if domain == "binary_sensor" {
			value = true
		}
		if err := json.NewEncoder(writer).Encode(map[string]any{"id": id, "value": value, "state": "12.5"}); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	targetDevice := device{server.URL, "127.0.0.1"}
	handler := newExporter(config{httpTimeout: time.Second}).handler()
	path := "/probe?target=" + url.QueryEscape(targetDevice.host)
	scrape := func() string {
		t.Helper()
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest("GET", path, nil))
		if response.Code != http.StatusOK || !strings.Contains(response.Header().Get("Content-Type"), "version=0.0.4") {
			t.Fatalf("unexpected scrape response: %d, %v", response.Code, response.Header())
		}
		return response.Body.String()
	}
	text := scrape()
	if !strings.Contains(text, `apollo_air1_device_up{device="127.0.0.1",host="`+server.URL+`"} 0`) {
		t.Fatalf("missing down metric: %s", text)
	}
	online.Store(true)
	text = scrape()
	parser := expfmt.NewTextParser(model.LegacyValidation)
	families, err := parser.TextToMetricFamilies(strings.NewReader(text))
	if err != nil {
		t.Fatal(err)
	}
	if len(families) != len(sensors)+1 {
		t.Fatalf("got %d metric families, want %d", len(families), len(sensors)+1)
	}
	for _, sensor := range sensors {
		family := families["apollo_air1_"+sensor.metric]
		count := 1
		if sensor.id == "voc_quality" {
			count = 6
		}
		if family == nil || len(family.Metric) != count {
			t.Fatalf("missing metric %s", sensor.metric)
		}
		for _, metric := range family.Metric {
			labels := make(map[string]string)
			for _, label := range metric.Label {
				labels[label.GetName()] = label.GetValue()
			}
			labelCount := 3
			if sensor.domain == "text_sensor" {
				labelCount = 4
			}
			if len(labels) != labelCount || labels["device"] != "127.0.0.1" || labels["host"] != server.URL || labels["sensor"] != sensor.name {
				t.Errorf("%s labels = %v, want device, host and sensor=%q", sensor.metric, labels, sensor.name)
			}
			want := 12.5
			if sensor.id == "rssi" {
				want = -62
			}
			if sensor.domain == "text_sensor" || sensor.domain == "binary_sensor" {
				want = 1
			}
			if sensor.id == "voc_quality" && labels["state"] != "Normal" {
				want = 0
			}
			if got := metric.GetGauge().GetValue(); got != want {
				t.Errorf("%s = %v, want %v", sensor.metric, got, want)
			}
		}
	}
	if families["apollo_air1_device_up"].Metric[0].GetGauge().GetValue() != 1 {
		t.Fatal("device failed to recover")
	}
	for _, label := range families["apollo_air1_device_up"].Metric[0].Label {
		if label.GetName() == "sensor" {
			t.Error("device_up must not have a sensor label")
		}
	}
	before := calls.Load()
	for _, test := range []struct {
		path, contains string
		code           int
	}{
		{"/health", "OK", 200}, {"/", "Apollo Air-1 Prometheus Exporter", 200},
		{"/missing", "404", 404},
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest("GET", test.path, nil))
		if response.Code != test.code || !strings.Contains(response.Body.String(), test.contains) {
			t.Errorf("%s: %d %q", test.path, response.Code, response.Body.String())
		}
	}
	if calls.Load() != before {
		t.Fatal("non-metrics endpoints triggered device requests")
	}
	online.Store(false)
	text = scrape()
	if strings.Contains(text, "apollo_air1_co2_ppm") || !strings.Contains(text, `apollo_air1_device_up{device="127.0.0.1",host="`+server.URL+`"} 0`) {
		t.Fatalf("failed scrape must report down without stale sensor values: %s", text)
	}
}

func TestPartialStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/sensor/CO2" {
			fmt.Fprint(writer, `{"id":"co2","value":450,"state":"450 ppm"}`)
			return
		}
		http.NotFound(writer, request)
	}))
	defer server.Close()
	values, err := newApolloClient(server.URL, time.Second).getStatus(context.Background())
	if err != nil || len(values) != 1 || values["co2"] != 450.0 {
		t.Fatalf("values=%v, error=%v", values, err)
	}
}

func TestSensorStateCleanupAndIsolation(t *testing.T) {
	metrics := newMetrics()
	other := newMetrics()
	target := device{"http://apollo", "Test Device"}
	second := device{"http://other", "Test Device"}
	third := device{"http://apollo", "Other Device"}
	for _, peer := range []device{second, third} {
		metrics.updateDevice(peer, map[string]any{"ip_address": "unchanged", "voc_quality": "Normal"})
	}
	for _, state := range []string{"Improved", "Normal", "Abnormal", "Very abnormal", "Extremely abnormal", "Unknown", "unexpected", ""} {
		metrics.updateDevice(target, map[string]any{
			"ip_address": state, "esphome_version": state, "apollo_firmware_version": state,
			"voc_quality": state, "online": false,
		})
		families, err := metrics.registry.Gather()
		if err != nil {
			t.Fatal(err)
		}
		for _, family := range families {
			count := 1
			switch family.GetName() {
			case "apollo_air1_device_up", "apollo_air1_ip_address_info":
				count = 3
			case "apollo_air1_voc_quality":
				count = 18
			}
			if len(family.Metric) != count {
				t.Fatalf("unexpected series count: %v", family)
			}
			for _, metric := range family.Metric {
				labels := make(map[string]string)
				for _, label := range metric.Label {
					labels[label.GetName()] = label.GetValue()
				}
				isTarget := labels["host"] == target.host && labels["device"] == target.name
				if strings.HasSuffix(family.GetName(), "_info") {
					want := "unchanged"
					if isTarget {
						want = state
					}
					if labels["value"] != want || metric.GetGauge().GetValue() != 1 {
						t.Fatalf("incorrect info state: %v", metric)
					}
				}
				if family.GetName() == "apollo_air1_voc_quality" {
					current := "Normal"
					if isTarget {
						current = state
						if state == "unexpected" || state == "" {
							current = "Unknown"
						}
					}
					want := 0.0
					if labels["state"] == current {
						want = 1
					}
					if metric.GetGauge().GetValue() != want {
						t.Fatalf("incorrect VOC enum state: %v", metric)
					}
				}
				if family.GetName() == "apollo_air1_online" && metric.GetGauge().GetValue() != 0 {
					t.Fatal("Online=false should be 0")
				}
			}
		}
	}
	metrics.updateDevice(target, map[string]any{"pm__2_5_m_weight_concentration": 100.0})
	text, err := metrics.gather()
	if err != nil || strings.Contains(text, "apollo_air1_aqi") {
		t.Fatalf("AQI must not be calculated from PM readings: %s, %v", text, err)
	}
	text, err = other.gather()
	if err != nil || text != "" {
		t.Fatalf("registries should be isolated: %q, %v", text, err)
	}
}

func TestConcurrentTargetScrapes(t *testing.T) {
	var calls atomic.Int32
	servers := make([]*httptest.Server, 2)
	for index := range servers {
		servers[index] = httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			calls.Add(1)
			if request.URL.Path != "/sensor/CO2" {
				http.NotFound(writer, request)
				return
			}
			fmt.Fprintf(writer, `{"id":"co2","value":%d,"state":"ppm"}`, index+400)
		}))
		defer servers[index].Close()
	}
	handler := newExporter(config{httpTimeout: 5 * time.Second}).handler()
	var workers sync.WaitGroup
	for index := range 20 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			target := index % len(servers)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest("GET", "/probe?target="+url.QueryEscape(servers[target].URL), nil))
			parser := expfmt.NewTextParser(model.LegacyValidation)
			families, err := parser.TextToMetricFamilies(strings.NewReader(response.Body.String()))
			if err != nil || response.Code != http.StatusOK || len(families) != 2 {
				t.Errorf("unexpected scrape response: %d, %s, %v", response.Code, response.Body.String(), err)
				return
			}
			family := families["apollo_air1_co2_ppm"]
			if family == nil || len(family.Metric) != 1 || family.Metric[0].GetGauge().GetValue() != float64(target+400) {
				t.Errorf("mixed target readings: %s", response.Body.String())
				return
			}
			for _, label := range family.Metric[0].Label {
				if label.GetName() == "host" && label.GetValue() != servers[target].URL {
					t.Errorf("mixed target host: %s", response.Body.String())
				}
			}
		}()
	}
	workers.Wait()
	if calls.Load() != int32(20*len(sensors)) {
		t.Fatalf("got %d device requests, want %d", calls.Load(), 20*len(sensors))
	}
}
