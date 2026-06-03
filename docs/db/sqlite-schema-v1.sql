-- Visiorama SQLite Schema V1
-- Date: 2026-06-03

PRAGMA foreign_keys = ON;
PRAGMA journal_mode = WAL;

CREATE TABLE IF NOT EXISTS albums (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  parent_album_id INTEGER NULL,
  relative_path TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  media_count_direct INTEGER NOT NULL DEFAULT 0,
  media_count_recursive INTEGER NOT NULL DEFAULT 0,
  child_album_count INTEGER NOT NULL DEFAULT 0,
  cover_media_id INTEGER NULL,
  dir_mtime_ns INTEGER NULL,
  created_at TEXT NOT NULL DEFAULT (datetime('now')),
  updated_at TEXT NOT NULL DEFAULT (datetime('now')),
  FOREIGN KEY(parent_album_id) REFERENCES albums(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS media (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  album_id INTEGER NOT NULL,
  relative_path TEXT NOT NULL UNIQUE,
  filename TEXT NOT NULL,
  media_type TEXT NOT NULL CHECK(media_type IN ('image', 'video')),
  extension TEXT NOT NULL,
  mime_type TEXT NOT NULL,
  size_bytes INTEGER NOT NULL,
  width INTEGER NULL,
  height INTEGER NULL,
  duration_ms INTEGER NULL,
  capture_date TEXT NULL,
  camera_model TEXT NULL,
  lens_model TEXT NULL,
  gps_lat REAL NULL,
  gps_lon REAL NULL,
  orientation INTEGER NULL,
  file_mtime_ns INTEGER NULL,
  warning_large_media INTEGER NOT NULL DEFAULT 0 CHECK(warning_large_media IN (0, 1)),
  thumb_status TEXT NOT NULL DEFAULT 'pending' CHECK(thumb_status IN ('pending', 'ready', 'error')),
  thumb_updated_at TEXT NULL,
  created_at TEXT NOT NULL DEFAULT (datetime('now')),
  updated_at TEXT NOT NULL DEFAULT (datetime('now')),
  FOREIGN KEY(album_id) REFERENCES albums(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS scan_jobs (
  id TEXT PRIMARY KEY,
  mode TEXT NOT NULL CHECK(mode IN ('full', 'quick')),
  status TEXT NOT NULL CHECK(status IN ('queued', 'running', 'success', 'failed')),
  started_at TEXT NULL,
  finished_at TEXT NULL,
  scanned_files INTEGER NOT NULL DEFAULT 0,
  indexed_files INTEGER NOT NULL DEFAULT 0,
  skipped_files INTEGER NOT NULL DEFAULT 0,
  error_count INTEGER NOT NULL DEFAULT 0,
  fallback_to_full INTEGER NOT NULL DEFAULT 0 CHECK(fallback_to_full IN (0, 1)),
  notes TEXT NULL,
  created_at TEXT NOT NULL DEFAULT (datetime('now')),
  updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS scan_job_errors (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  scan_job_id TEXT NOT NULL,
  relative_path TEXT NULL,
  error_code TEXT NOT NULL,
  error_message TEXT NOT NULL,
  created_at TEXT NOT NULL DEFAULT (datetime('now')),
  FOREIGN KEY(scan_job_id) REFERENCES scan_jobs(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS fs_state (
  relative_path TEXT PRIMARY KEY,
  entry_type TEXT NOT NULL CHECK(entry_type IN ('file', 'dir')),
  last_mtime_ns INTEGER NOT NULL,
  last_size_bytes INTEGER NULL,
  last_seen_scan_job_id TEXT NULL,
  updated_at TEXT NOT NULL DEFAULT (datetime('now')),
  FOREIGN KEY(last_seen_scan_job_id) REFERENCES scan_jobs(id) ON DELETE SET NULL
);

CREATE TABLE IF NOT EXISTS service_config (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL,
  updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_albums_parent ON albums(parent_album_id);
CREATE INDEX IF NOT EXISTS idx_albums_name ON albums(name);
CREATE INDEX IF NOT EXISTS idx_media_album ON media(album_id);
CREATE INDEX IF NOT EXISTS idx_media_type ON media(media_type);
CREATE INDEX IF NOT EXISTS idx_media_capture_date ON media(capture_date);
CREATE INDEX IF NOT EXISTS idx_media_filename ON media(filename);
CREATE INDEX IF NOT EXISTS idx_scan_jobs_status ON scan_jobs(status);
CREATE INDEX IF NOT EXISTS idx_scan_jobs_started ON scan_jobs(started_at);
CREATE INDEX IF NOT EXISTS idx_scan_errors_job ON scan_job_errors(scan_job_id);
CREATE INDEX IF NOT EXISTS idx_fs_state_type ON fs_state(entry_type);
CREATE INDEX IF NOT EXISTS idx_fs_state_mtime ON fs_state(last_mtime_ns);
