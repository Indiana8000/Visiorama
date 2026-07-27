package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/Indiana8000/visiorama/internal/app"
	"github.com/Indiana8000/visiorama/internal/index"
	"github.com/Indiana8000/visiorama/internal/scan"
)

// runScan handles `visiorama scan --mode full|quick|orphan [--config path]`.
// It runs the scanner synchronously and prints the result, without starting
// the HTTP server or the async job-queue bookkeeping used by the API.
func runScan(args []string) error {
	fs := flag.NewFlagSet("scan", flag.ExitOnError)
	cfgPath := fs.String("config", "configs/visiorama.yaml", "path to config file")
	mode := fs.String("mode", "", "scan mode: full, quick, or orphan (default: scan.defaultMode from config)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := app.LoadConfig(*cfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}

	effectiveMode := *mode
	if effectiveMode == "" {
		effectiveMode = cfg.Scan.DefaultMode
	}
	if effectiveMode != "full" && effectiveMode != "quick" && effectiveMode != "orphan" {
		return fmt.Errorf("mode must be 'full', 'quick' or 'orphan', got %q", effectiveMode)
	}

	store, err := index.Open(cfg.Database.SQLitePath)
	if err != nil {
		return fmt.Errorf("open index: %w", err)
	}
	defer store.Close()

	if err := index.Migrate(store); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	ctx := context.Background()
	const scanID = "cli-scan"

	var stats *scan.Stats
	switch effectiveMode {
	case "full":
		stats, err = scan.NewFullScanner(cfg, store).Run(ctx, scanID)
	case "quick":
		var fallback bool
		stats, fallback, err = scan.NewQuickScanner(cfg, store).Run(ctx, scanID)
		if fallback {
			fmt.Println("quick scan fell back to full scan")
		}
	case "orphan":
		stats, err = scan.NewOrphanScanner(cfg, store).Run(ctx, scanID)
	}
	if stats != nil {
		fmt.Printf("scan complete: mode=%s scanned=%d indexed=%d skipped=%d errors=%d\n",
			effectiveMode, stats.Scanned.Load(), stats.Indexed.Load(), stats.Skipped.Load(), stats.ErrCount.Load())
	}
	if err != nil {
		return fmt.Errorf("%s scan: %w", effectiveMode, err)
	}
	return nil
}
