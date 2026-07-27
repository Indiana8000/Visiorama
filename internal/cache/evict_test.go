package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeFileAt(t *testing.T, path string, size int, mtime time.Time) {
	t.Helper()
	if err := os.WriteFile(path, make([]byte, size), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}
}

func TestEvictToBudget_RemovesOldestFirst(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()

	oldest := filepath.Join(dir, "oldest.jpg")
	middle := filepath.Join(dir, "middle.jpg")
	newest := filepath.Join(dir, "newest.jpg")

	writeFileAt(t, oldest, 100, now.Add(-3*time.Hour))
	writeFileAt(t, middle, 100, now.Add(-2*time.Hour))
	writeFileAt(t, newest, 100, now.Add(-1*time.Hour))

	// Budget fits only the newest 2 files (200 bytes).
	removed, freed, err := EvictToBudget(dir, 200)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 1 || freed != 100 {
		t.Fatalf("removed=%d freed=%d, want removed=1 freed=100", removed, freed)
	}
	if _, err := os.Stat(oldest); !os.IsNotExist(err) {
		t.Error("oldest file should have been evicted")
	}
	if _, err := os.Stat(middle); err != nil {
		t.Error("middle file should still exist")
	}
	if _, err := os.Stat(newest); err != nil {
		t.Error("newest file should still exist")
	}
}

func TestEvictToBudget_NoopUnderBudget(t *testing.T) {
	dir := t.TempDir()
	writeFileAt(t, filepath.Join(dir, "a.jpg"), 100, time.Now())

	removed, freed, err := EvictToBudget(dir, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 0 || freed != 0 {
		t.Fatalf("removed=%d freed=%d, want 0/0 when under budget", removed, freed)
	}
}

func TestEvictToBudget_ZeroDisablesEnforcement(t *testing.T) {
	dir := t.TempDir()
	writeFileAt(t, filepath.Join(dir, "a.jpg"), 100, time.Now())

	removed, freed, err := EvictToBudget(dir, 0)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 0 || freed != 0 {
		t.Fatalf("removed=%d freed=%d, want 0/0 when maxBytes<=0", removed, freed)
	}
}

func TestSafeDir_RejectsRelativeAndShallow(t *testing.T) {
	if err := SafeDir("relative/path"); err == nil {
		t.Error("expected error for relative path")
	}
	if err := SafeDir(""); err == nil {
		t.Error("expected error for empty path")
	}
}
