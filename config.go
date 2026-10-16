package main

import (
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/prometheus/exporter-toolkit/web"
	"github.com/prometheus/exporter-toolkit/web/kingpinflag"
)

var errHelpRequested = errors.New("help or version requested")

type device struct {
	host, name string
}

type config struct {
	bind               string
	port               int
	httpTimeout        time.Duration
	logLevel           string
	webConfigFile      string
	webListenAddresses []string
	webFlags           *web.FlagConfig
}

func parseConfig(args []string, getenv func(string) string, output io.Writer) (config, error) {
	var cfg config
	value := func(key, fallback string) string {
		if result := getenv(key); result != "" {
			return result
		}
		return fallback
	}
	bindDefault := value("APOLLO_EXPORTER_BIND", "0.0.0.0")
	portDefault := value("APOLLO_EXPORTER_PORT", "9926")
	timeoutDefault := value("APOLLO_HTTP_TIMEOUT", "10")
	logLevelDefault := value("APOLLO_LOG_LEVEL", "info")
	app := kingpin.New("apollo_air1_exporter", "Prometheus exporter for Apollo Air-1 air quality monitors.").Writer(output)
	terminated := false
	app.Terminate(func(code int) {
		terminated = code == 0
	})
	app.HelpFlag.Short('h')
	port := app.Flag("port", "Metrics port (APOLLO_EXPORTER_PORT)").Short('p').Default(portDefault).Int()
	bind := app.Flag("bind", "Metrics bind address (APOLLO_EXPORTER_BIND)").Default(bindDefault).String()
	timeout := app.Flag("http-timeout", "Device collection timeout per scrape in seconds (APOLLO_HTTP_TIMEOUT)").Default(timeoutDefault).String()
	logLevel := app.Flag("log-level", "Log level: trace, debug, info, warn, error (APOLLO_LOG_LEVEL)").Default(logLevelDefault).Enum("trace", "debug", "info", "warn", "error")
	app.Version(fmt.Sprintf("apollo_air1_exporter %s", version)).VersionFlag.Short('V')
	webFlags := kingpinflag.AddFlags(app, net.JoinHostPort(bindDefault, portDefault))
	app.GetFlag("web.config.file").Default(value("APOLLO_WEB_CONFIG_FILE", ""))
	var listenAddressSet bool
	app.GetFlag("web.listen-address").IsSetByUser(&listenAddressSet)
	if _, err := app.Parse(args); err != nil {
		return cfg, err
	}
	if terminated {
		return cfg, errHelpRequested
	}
	if *port < 0 || *port > 65535 {
		return cfg, fmt.Errorf("invalid port: %q", strconv.Itoa(*port))
	}
	cfg.port = *port
	cfg.bind = *bind
	seconds, err := strconv.ParseInt(*timeout, 10, 64)
	if err != nil {
		return cfg, fmt.Errorf("HTTP timeout must be a positive number of seconds")
	}
	if seconds <= 0 || seconds > int64((1<<63-1)/time.Second) {
		return cfg, fmt.Errorf("HTTP timeout must be a positive number of seconds")
	}
	cfg.httpTimeout = time.Duration(seconds) * time.Second
	cfg.logLevel = *logLevel
	if !listenAddressSet {
		*webFlags.WebListenAddresses = []string{cfg.address()}
	}
	for _, address := range *webFlags.WebListenAddresses {
		if address == "" {
			return cfg, fmt.Errorf("listen address cannot be empty")
		}
	}
	cfg.webListenAddresses = *webFlags.WebListenAddresses
	cfg.webConfigFile = *webFlags.WebConfigFile
	cfg.webFlags = webFlags
	return cfg, nil
}

func (cfg config) address() string {
	return net.JoinHostPort(cfg.bind, strconv.Itoa(cfg.port))
}
