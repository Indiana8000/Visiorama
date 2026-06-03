package main

import (
	"flag"
	"log/slog"
	"os"

	"github.com/USERNAME/visiorama/internal/app"
	"github.com/USERNAME/visiorama/internal/server"
)

func main() {
	cfgPath := flag.String("config", "configs/visiorama.yaml", "path to config file")
	flag.Parse()

	cfg, err := app.LoadConfig(*cfgPath)
	if err != nil {
		slog.Error("load config", "err", err)
		os.Exit(1)
	}
	if err := cfg.Validate(); err != nil {
		slog.Error("invalid config", "err", err)
		os.Exit(1)
	}

	if err := server.Run(cfg); err != nil {
		slog.Error("run", "err", err)
		os.Exit(1)
	}
}
