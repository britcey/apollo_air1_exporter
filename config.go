package main

import (
	"flag"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"
)

type device struct {
	host, name string
}

type config struct {
	bind        string
	port        int
	httpTimeout time.Duration
	logLevel    string
}

func parseConfig(args []string, getenv func(string) string, output io.Writer) (config, error) {
	var cfg config
	value := func(key, fallback string) string {
		if result := getenv(key); result != "" {
			return result
		}
		return fallback
	}
	flags := flag.NewFlagSet("apollo_air1_exporter", flag.ContinueOnError)
	flags.SetOutput(output)
	port := value("APOLLO_EXPORTER_PORT", "9926")
	flags.StringVar(&port, "port", port, "Metrics port (APOLLO_EXPORTER_PORT)")
	flags.StringVar(&port, "p", port, "Metrics port (shorthand)")
	flags.StringVar(&cfg.bind, "bind", value("APOLLO_EXPORTER_BIND", "0.0.0.0"), "Metrics bind address (APOLLO_EXPORTER_BIND)")
	timeout := flags.String("http-timeout", value("APOLLO_HTTP_TIMEOUT", "10"), "Device collection timeout per scrape in seconds (APOLLO_HTTP_TIMEOUT)")
	flags.StringVar(&cfg.logLevel, "log-level", value("APOLLO_LOG_LEVEL", "info"), "Log level: trace, debug, info, warn, error (APOLLO_LOG_LEVEL)")
	showVersion := flags.Bool("version", false, "Print version")
	flags.BoolVar(showVersion, "V", false, "Print version (shorthand)")
	if err := flags.Parse(args); err != nil {
		return cfg, err
	}
	if *showVersion {
		fmt.Fprintln(output, "apollo_air1_exporter", version)
		return cfg, flag.ErrHelp
	}
	if flags.NArg() != 0 {
		return cfg, fmt.Errorf("unexpected argument: %s", flags.Arg(0))
	}
	parsedPort, err := strconv.ParseUint(port, 10, 16)
	if err != nil {
		return cfg, fmt.Errorf("invalid port: %q", port)
	}
	cfg.port = int(parsedPort)
	seconds, err := strconv.ParseInt(*timeout, 10, 64)
	if err != nil || seconds <= 0 || seconds > int64((1<<63-1)/time.Second) {
		return cfg, fmt.Errorf("HTTP timeout must be a positive number of seconds")
	}
	cfg.httpTimeout = time.Duration(seconds) * time.Second
	switch cfg.logLevel {
	case "trace", "debug", "info", "warn", "error":
	default:
		return cfg, fmt.Errorf("invalid log level: %q", cfg.logLevel)
	}
	return cfg, nil
}

func (cfg config) address() string {
	return net.JoinHostPort(cfg.bind, strconv.Itoa(cfg.port))
}
