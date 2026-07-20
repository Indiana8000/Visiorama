package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/Indiana8000/visiorama/internal/ai"
)

var version = "dev"

func main() {
	socketPath := flag.String("socket", "", "Unix socket path (default: /tmp/visiorama-ai.sock)")
	modelDir   := flag.String("models", "", "Directory for ONNX model storage")
	cropsDir   := flag.String("crops", "", "Directory for face crop JPEGs")
	workers    := flag.Int("workers", 0, "Inference worker count (0 = auto)")
	flag.Parse()

	if *socketPath == "" {
		*socketPath = ai.DefaultSocketPath()
	}
	if *modelDir == "" {
		home, _ := os.UserCacheDir()
		*modelDir = filepath.Join(home, "visiorama", "models")
	}
	if *cropsDir == "" {
		*cropsDir = filepath.Join(filepath.Dir(*modelDir), "crops")
	}
	if *workers <= 0 {
		*workers = 2
	}

	if err := os.MkdirAll(*modelDir, 0755); err != nil {
		slog.Error("create model dir", "err", err)
		os.Exit(1)
	}

	mgr := newModelManager(*modelDir, *cropsDir)
	if err := mgr.EnsureModels(context.Background()); err != nil {
		slog.Error("model setup failed", "err", err)
		os.Exit(1)
	}

	if err := os.MkdirAll(*cropsDir, 0755); err != nil {
		slog.Error("create crops dir", "err", err)
		os.Exit(1)
	}

	srv := &server{
		modelDir: *modelDir,
		cropsDir: *cropsDir,
		workers:  *workers,
		mgr:      mgr,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health",    srv.handleHealth)
	mux.HandleFunc("POST /analyze",  srv.handleAnalyze)

	// Remove stale socket if it exists.
	_ = os.Remove(*socketPath)
	ln, err := net.Listen("unix", *socketPath)
	if err != nil {
		slog.Error("listen unix socket", "socket", *socketPath, "err", err)
		os.Exit(1)
	}
	defer os.Remove(*socketPath)

	httpSrv := &http.Server{
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 10 * time.Minute,
		IdleTimeout:  120 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		slog.Info("visiorama-ai listening", "socket", *socketPath, "workers", *workers, "version", version)
		if err := httpSrv.Serve(ln); err != nil && err != http.ErrServerClosed {
			slog.Error("serve", "err", err)
			stop()
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down")
	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(shutCtx)
}

type server struct {
	modelDir string
	cropsDir string
	workers  int
	mgr      *modelManager
}

func (s *server) handleHealth(w http.ResponseWriter, r *http.Request) {
	resp := ai.StatusResponse{
		Available:    true,
		Version:      version,
		LoadedModels: s.mgr.LoadedModels(),
		Workers:      s.workers,
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *server) handleAnalyze(w http.ResponseWriter, r *http.Request) {
	var req ai.AnalyzeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("bad request: %v", err), http.StatusBadRequest)
		return
	}
	if req.FilePath == "" {
		http.Error(w, "filePath required", http.StatusBadRequest)
		return
	}

	result, err := s.mgr.Analyze(r.Context(), req)
	if err != nil {
		slog.Warn("analyze failed", "mediaId", req.MediaID, "err", err)
		http.Error(w, fmt.Sprintf("analyze: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}
