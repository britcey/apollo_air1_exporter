package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	promversion "github.com/prometheus/common/version"
	"github.com/prometheus/exporter-toolkit/web"
)

type exporter struct {
	httpTimeout time.Duration
}

func newExporter(cfg config) *exporter {
	return &exporter{httpTimeout: cfg.httpTimeout}
}

func (exporter *exporter) handler() http.Handler {
	mux := http.NewServeMux()
	exporterRegistry := prometheus.NewRegistry()
	exporterRegistry.MustRegister(prometheus.NewGoCollector(), prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
	info := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "apollo_air1_exporter_info",
		Help: "Build information for the Apollo Air-1 exporter.",
		ConstLabels: prometheus.Labels{
			"version":    promversion.Version,
			"revision":   promversion.Revision,
			"branch":     promversion.Branch,
			"build_user": promversion.BuildUser,
			"build_date": promversion.BuildDate,
			"go_version": promversion.GoVersion,
			"goos":       promversion.GoOS,
			"goarch":     promversion.GoArch,
		},
	})
	info.Set(1)
	exporterRegistry.MustRegister(info)
	mux.Handle("GET /metrics", promhttp.HandlerFor(exporterRegistry, promhttp.HandlerOpts{}))
	mux.HandleFunc("GET /probe", func(writer http.ResponseWriter, request *http.Request) {
		targets := request.URL.Query()["target"]
		if len(targets) != 1 || targets[0] == "" {
			http.Error(writer, "exactly one target query parameter is required", http.StatusBadRequest)
			return
		}
		target, err := url.Parse(targets[0])
		if err != nil || (target.Scheme != "http" && target.Scheme != "https") || target.Hostname() == "" || target.User != nil || target.RawQuery != "" || target.Fragment != "" {
			http.Error(writer, "target must be an http:// or https:// device URL without credentials, query or fragment", http.StatusBadRequest)
			return
		}
		device := device{targets[0], target.Hostname()}
		ctx, cancel := context.WithTimeout(request.Context(), exporter.httpTimeout)
		defer cancel()
		metrics := newMetrics()
		values, err := newApolloClient(device.host, exporter.httpTimeout).getStatus(ctx)
		if err != nil {
			slog.Warn("Failed to fetch device status", "device", device.name, "host", device.host, "error", err)
			metrics.markDeviceDown(device)
		} else {
			metrics.updateDevice(device, values)
			slog.Debug("Retrieved device sensors", "device", device.name, "count", len(values))
		}
		text, err := metrics.gather()
		if err != nil {
			slog.Error("Failed to gather metrics", "error", err)
			http.Error(writer, "failed to gather metrics", http.StatusInternalServerError)
			return
		}
		writer.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		fmt.Fprint(writer, text)
	})
	mux.HandleFunc("GET /health", func(writer http.ResponseWriter, request *http.Request) {
		fmt.Fprint(writer, "OK")
	})
	mux.HandleFunc("GET /{$}", func(writer http.ResponseWriter, request *http.Request) {
		fmt.Fprint(writer, "Apollo Air-1 Prometheus Exporter\n\nEndpoints:\n  /probe?target=http://device - Query device metrics\n  /metrics - Exporter metrics\n  /health - Health check\n")
	})
	return mux
}

func run() error {
	cfg, err := parseConfig(os.Args[1:], os.Getenv, os.Stdout)
	if errors.Is(err, errHelpRequested) {
		return nil
	}
	if err != nil {
		return err
	}
	level := slog.LevelInfo
	switch cfg.logLevel {
	case "trace", "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	exporter := newExporter(cfg)
	server := &http.Server{Handler: exporter.handler(), ReadHeaderTimeout: 5 * time.Second}
	systemdSocket := false
	webFlags := &web.FlagConfig{
		WebListenAddresses: &cfg.webListenAddresses,
		WebSystemdSocket:   &systemdSocket,
		WebConfigFile:      &cfg.webConfigFile,
	}
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			slog.Error("HTTP shutdown failed", "error", err)
		}
	}()
	logArgs := append(promversion.Slog(), "addresses", cfg.webListenAddresses)
	slog.Info("Starting Apollo Air-1 Prometheus Exporter", logArgs...)
	err = web.ListenAndServe(server, webFlags, slog.Default())
	stop()
	<-shutdownDone
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
