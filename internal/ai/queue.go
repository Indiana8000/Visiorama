package ai

import (
	"context"
	"log/slog"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Indiana8000/visiorama/internal/app"
	"github.com/Indiana8000/visiorama/internal/index/repositories"
)

const (
	maxAttempts    = 3
	workerInterval = 2 * time.Second // poll interval when queue is empty
	startupTimeout = 15 * time.Second
)

// QueueStats holds live counters exposed via /api/ai/status.
type QueueStats struct {
	Queued  atomic.Int64
	Running atomic.Int64
	Done    atomic.Int64
	Failed  atomic.Int64
}

// QueueRunner processes ai_jobs rows using the visiorama-ai sidecar.
type QueueRunner struct {
	cfg    *app.Config
	repo   *repositories.AIRepo
	client *Client // may be nil initially; set once sidecar is reachable
	mu     sync.Mutex
	stats  QueueStats
}

func NewQueueRunner(cfg *app.Config, repo *repositories.AIRepo, client *Client) *QueueRunner {
	return &QueueRunner{cfg: cfg, repo: repo, client: client}
}

// EnqueueScan implements scan.AIEnqueuer — called by scan.Runner after indexing.
func (q *QueueRunner) EnqueueScan(mediaIDs []int64) {
	now := time.Now().UTC().Format(time.RFC3339)
	if err := q.repo.EnqueueNew(mediaIDs, now); err != nil {
		slog.Warn("ai queue: enqueue after scan failed", "err", err, "count", len(mediaIDs))
		return
	}
	slog.Info("ai queue: enqueued after scan", "count", len(mediaIDs))
	q.stats.Queued.Add(int64(len(mediaIDs)))
}

// Start launches worker goroutines. Blocks until ctx is cancelled.
func (q *QueueRunner) Start(ctx context.Context) {
	workers := q.cfg.AI.Workers
	if workers <= 0 {
		workers = 2
	}
	slog.Info("ai queue runner started", "workers", workers)

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			q.runWorker(ctx)
		}()
	}
	wg.Wait()
}

func (q *QueueRunner) Stats() *QueueStats { return &q.stats }

// SetClient updates the sidecar client (called after auto-start succeeds).
func (q *QueueRunner) SetClient(c *Client) {
	q.mu.Lock()
	q.client = c
	q.mu.Unlock()
}

func (q *QueueRunner) client_() *Client {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.client
}

func (q *QueueRunner) runWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		c := q.client_()
		if c == nil {
			c = q.tryStartSidecar(ctx)
			if c == nil {
				select {
				case <-ctx.Done():
					return
				case <-time.After(workerInterval):
				}
				continue
			}
		}

		job, err := q.repo.ClaimNext()
		if err != nil {
			slog.Warn("ai queue: claim failed", "err", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(workerInterval):
			}
			continue
		}
		if job == nil {
			select {
			case <-ctx.Done():
				return
			case <-time.After(workerInterval):
			}
			continue
		}

		q.stats.Running.Add(1)
		q.processJob(ctx, c, job)
		q.stats.Running.Add(-1)
	}
}

func (q *QueueRunner) processJob(ctx context.Context, c *Client, job *repositories.AIJob) {
	absPath, mediaType, err := q.repo.GetMediaPath(job.MediaID, q.cfg.Library.RootPath)
	if err != nil {
		slog.Warn("ai queue: media not found", "mediaId", job.MediaID, "err", err)
		q.finishJob(job.MediaID, false, err.Error())
		return
	}

	jobCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	result, err := c.Analyze(jobCtx, AnalyzeRequest{
		MediaID:   job.MediaID,
		FilePath:  absPath,
		MediaType: mediaType,
	})
	if err != nil {
		slog.Warn("ai queue: analyze failed", "mediaId", job.MediaID, "attempt", job.Attempts, "err", err)
		q.finishJob(job.MediaID, false, err.Error())
		return
	}

	minConf := q.cfg.AI.LabelMinConfidence
	if minConf <= 0 {
		minConf = 0.6
	}
	minFacePx := q.cfg.AI.FaceMinPixels
	if minFacePx <= 0 {
		minFacePx = 40
	}

	// Persist labels.
	labels := make([]repositories.AILabel, 0, len(result.Labels))
	for _, l := range result.Labels {
		if l.Confidence >= minConf {
			labels = append(labels, repositories.AILabel{
				MediaID:    job.MediaID,
				Label:      l.Label,
				Confidence: l.Confidence,
				Source:     l.Source,
			})
		}
	}
	if err := q.repo.SaveLabels(job.MediaID, labels); err != nil {
		slog.Warn("ai queue: save labels failed", "mediaId", job.MediaID, "err", err)
	}

	// Persist faces.
	faces := make([]repositories.AIFace, 0, len(result.Faces))
	for _, f := range result.Faces {
		if f.BBox.W < minFacePx || f.BBox.H < minFacePx {
			continue
		}
		faces = append(faces, repositories.AIFace{
			MediaID:   job.MediaID,
			BBoxJSON:  repositories.BBoxToJSON(f.BBox.X, f.BBox.Y, f.BBox.W, f.BBox.H),
			Embedding: repositories.F32ToBlob(f.Embedding),
			CropPath:  f.CropPath,
		})
	}
	if err := q.repo.SaveFaces(job.MediaID, faces); err != nil {
		slog.Warn("ai queue: save faces failed", "mediaId", job.MediaID, "err", err)
	}

	slog.Info("ai queue: analyzed",
		"mediaId", job.MediaID,
		"labels", len(labels),
		"faces", len(faces),
	)
	q.finishJob(job.MediaID, true, "")
	q.stats.Done.Add(1)
}

func (q *QueueRunner) finishJob(mediaID int64, success bool, errMsg string) {
	now := time.Now().UTC().Format(time.RFC3339)
	if err := q.repo.Finish(mediaID, success, errMsg, now); err != nil {
		slog.Warn("ai queue: finish job failed", "mediaId", mediaID, "err", err)
	}
	if !success {
		q.stats.Failed.Add(1)
	}
}

// tryStartSidecar attempts to start the visiorama-ai binary if configured.
// Returns a reachable client or nil.
func (q *QueueRunner) tryStartSidecar(ctx context.Context) *Client {
	socketPath := q.cfg.AI.SocketPath
	if socketPath == "" {
		socketPath = "/tmp/visiorama-ai.sock"
	}

	// Ping first — may already be running.
	probe := NewClient(socketPath)
	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := probe.Ping(pingCtx); err == nil {
		q.SetClient(probe)
		slog.Info("ai queue: sidecar reconnected", "socket", socketPath)
		return probe
	}

	// Not reachable — try to spawn it.
	binPath := BinaryPath(q.cfg.AI.Binary)
	if binPath == "" {
		return nil
	}

	modelDir := q.cfg.AI.ModelDir
	if modelDir == "" {
		return nil
	}

	cropsDir := q.cfg.AI.FaceCacheDir

	slog.Info("ai queue: starting sidecar", "binary", binPath, "socket", socketPath)
	args := []string{"--socket", socketPath, "--models", modelDir}
	if cropsDir != "" {
		args = append(args, "--crops", cropsDir)
	}
	cmd := exec.CommandContext(ctx, binPath, args...)
	if err := cmd.Start(); err != nil {
		slog.Warn("ai queue: sidecar start failed", "err", err)
		return nil
	}

	// Wait until socket is reachable.
	deadline := time.Now().Add(startupTimeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(500 * time.Millisecond):
		}
		pingCtx2, cancel2 := context.WithTimeout(ctx, time.Second)
		err := probe.Ping(pingCtx2)
		cancel2()
		if err == nil {
			q.SetClient(probe)
			slog.Info("ai queue: sidecar ready", "socket", socketPath)
			return probe
		}
	}
	slog.Warn("ai queue: sidecar did not become ready in time", "socket", socketPath)
	return nil
}
