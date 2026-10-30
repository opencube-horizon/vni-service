package main

import (
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
)

var (
	version   string
	buildTime string
)

func main() {
	filePath := flag.String("file", "/opt/db/db.sqlite3", "Path to sqlite3 file")
	port := flag.Int("port", 8842, "Port to listen on")
	vniMin := flag.Int("vni_min", 100, "VNI range start")
	vniMax := flag.Int("vni_max", 65535, "VNI range end")

	shouldLog := flag.Bool("vni_allocs_log", false, "Log events to vni_allocs_log")
	flag.Func("log_level", "Log level, one of debug, info, warn, error (default: info)", func(value string) error {
		switch strings.ToLower(value) {
		case "debug":
			slog.SetLogLoggerLevel(slog.LevelDebug)
			break
		case "info":
			slog.SetLogLoggerLevel(slog.LevelInfo)
			break
		case "warn":
			slog.SetLogLoggerLevel(slog.LevelWarn)
			break
		case "error":
			slog.SetLogLoggerLevel(slog.LevelError)
			break

		default:
			return errors.New(fmt.Sprintf("unknown log level %s, defaulting to info", value))
		}
		return nil
	})
	flag.Parse()

	slog.Info(fmt.Sprintf("Starting VNI Endpoint Service Version %s, build time %s", version, buildTime))
	if err := StartServer(filePath, *shouldLog, *port, *vniMin, *vniMax); err != nil {
		slog.Error("Error starting server", "err", err)
		os.Exit(1)
	}
}
