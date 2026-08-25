package main

import (
	"flag"
	"fmt"
	"time"

	"github.com/Indiana8000/visiorama/internal/app"
	"github.com/Indiana8000/visiorama/internal/index"
	"github.com/Indiana8000/visiorama/internal/index/repositories"
)

// runAIScan handles `visiorama ai-scan --mode full|quick [--config path] [--yes]`.
// It only queues ai_jobs rows; a visiorama server process with AI workers must
// already be running against the same database and sidecar socket to actually
// process them.
func runAIScan(args []string) error {
	fs := flag.NewFlagSet("ai-scan", flag.ExitOnError)
	cfgPath := fs.String("config", "configs/visiorama.yaml", "path to config file")
	mode := fs.String("mode", "", "ai-scan mode: quick or full (required)")
	yes := fs.Bool("yes", false, "confirm full re-analysis of the entire library (required for -mode full)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *mode != "full" && *mode != "quick" {
		return fmt.Errorf("mode must be 'full' or 'quick', got %q", *mode)
	}

	cfg, err := app.LoadConfig(*cfgPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}
	if cfg.AI.ModelDir == "" {
		return fmt.Errorf("ai.modelDir is not set in config — AI is not configured on this install, queuing jobs would have nothing to process them")
	}

	store, err := index.Open(cfg.Database.SQLitePath)
	if err != nil {
		return fmt.Errorf("open index: %w", err)
	}
	defer store.Close()

	if err := index.Migrate(store); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	mediaRepo := repositories.NewMediaRepo(store.DB())
	aiRepo := repositories.NewAIRepo(store.DB())

	ids, err := mediaRepo.ListAllIDs()
	if err != nil {
		return fmt.Errorf("list media: %w", err)
	}

	now := time.Now().UTC().Format(time.RFC3339)

	if *mode == "full" {
		if !*yes {
			fmt.Printf("would queue %d media items for full re-analysis (including already-successful ones)\n", len(ids))
			fmt.Println("pass -yes to confirm")
			return nil
		}
		if err := aiRepo.EnqueueAll(now); err != nil {
			return fmt.Errorf("enqueue all: %w", err)
		}
		fmt.Printf("queued %d media items for full re-analysis\n", len(ids))
		return nil
	}

	if err := aiRepo.EnqueueNew(ids, now); err != nil {
		return fmt.Errorf("enqueue new: %w", err)
	}
	fmt.Printf("checked %d media items; new and previously-failed jobs re-queued for analysis\n", len(ids))
	return nil
}
