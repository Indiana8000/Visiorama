package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

// DefaultSocketPath returns the platform-appropriate default Unix socket path.
func DefaultSocketPath() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(os.TempDir(), "visiorama-ai.sock")
	}
	return "/tmp/visiorama-ai.sock"
}

// Client communicates with a running visiorama-ai sidecar over a Unix socket.
type Client struct {
	http       *http.Client
	baseURL    string
	socketPath string
}

// NewClient creates a client that connects to the visiorama-ai sidecar via Unix socket.
func NewClient(socketPath string) *Client {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
		},
	}
	return &Client{
		http:       &http.Client{Transport: transport, Timeout: 120 * time.Second},
		baseURL:    "http://visiorama-ai",
		socketPath: socketPath,
	}
}

// Ping returns nil if the sidecar is reachable and healthy.
func (c *Client) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/health", nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("sidecar health: %s", resp.Status)
	}
	return nil
}

// Status returns the sidecar's current status.
func (c *Client) Status(ctx context.Context) (*StatusResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/health", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var s StatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		return nil, err
	}
	return &s, nil
}

// Analyze sends one media item to the sidecar for analysis and returns results.
func (c *Client) Analyze(ctx context.Context, req AnalyzeRequest) (*AnalyzeResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/analyze", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("sidecar analyze: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sidecar analyze: HTTP %s", resp.Status)
	}
	var result AnalyzeResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

// BinaryAvailable returns true if the visiorama-ai binary is found at the given path
// or (if path is empty) anywhere in PATH.
func BinaryAvailable(binaryPath string) bool {
	if binaryPath != "" {
		_, err := exec.LookPath(binaryPath)
		return err == nil
	}
	_, err := exec.LookPath("visiorama-ai")
	return err == nil
}

// BinaryPath resolves the full path to visiorama-ai. Returns empty string if not found.
func BinaryPath(configured string) string {
	if configured != "" {
		if p, err := exec.LookPath(configured); err == nil {
			return p
		}
		return ""
	}
	p, _ := exec.LookPath("visiorama-ai")
	return p
}
