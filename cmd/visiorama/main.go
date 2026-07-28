package main

import (
	"flag"
	"log/slog"
	"os"

	"github.com/Indiana8000/visiorama/internal/app"
	"github.com/Indiana8000/visiorama/internal/server"
)

var version = "dev"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "scan" {
		if err := runScan(os.Args[2:]); err != nil {
			slog.Error("scan", "err", err)
			os.Exit(1)
		}
		return
	}

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

	if err := server.Run(cfg, version); err != nil {
		slog.Error("run", "err", err)
		os.Exit(1)
	}
}
