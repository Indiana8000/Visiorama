package index

import (
	"fmt"
	"strings"
)

const schema = `
CREATE TABLE IF NOT EXISTS albums (
    id                   INTEGER PRIMARY KEY AUTOINCREMENT,
    relative_path        TEXT    NOT NULL UNIQUE,
    name                 TEXT    NOT NULL,
    parent_album_id      INTEGER REFERENCES albums(id),
    media_count_direct   INTEGER NOT NULL DEFAULT 0,
    media_count_recursive INTEGER NOT NULL DEFAULT 0,
    child_album_count    INTEGER NOT NULL DEFAULT 0,
    dir_mtime_ns         INTEGER
);

CREATE TABLE IF NOT EXISTS media (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    album_id        INTEGER NOT NULL REFERENCES albums(id),
    filename        TEXT    NOT NULL,
    relative_path   TEXT    NOT NULL UNIQUE,
    type            TEXT    NOT NULL CHECK(type IN ('image','video')),
    width           INTEGER,
    height          INTEGER,
    duration_ms     INTEGER,
    size_bytes      INTEGER NOT NULL,
    capture_date    TEXT,
    extension       TEXT    NOT NULL,
    mime_type       TEXT    NOT NULL,
    camera_model    TEXT,
    lens_model      TEXT,
    gps_lat         REAL,
    gps_lon         REAL,
    orientation     INTEGER,
    mtime_unix      INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS scan_jobs (
    id              TEXT    PRIMARY KEY,
    mode            TEXT    NOT NULL CHECK(mode IN ('full','quick','orphan')),
    status          TEXT    NOT NULL CHECK(status IN ('queued','running','success','failed')),
    started_at      TEXT,
    finished_at     TEXT,
    scanned_files   INTEGER NOT NULL DEFAULT 0,
    indexed_files   INTEGER NOT NULL DEFAULT 0,
    skipped_files   INTEGER NOT NULL DEFAULT 0,
    error_count     INTEGER NOT NULL DEFAULT 0,
    fallback_to_full INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS scan_errors (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    scan_id     TEXT    NOT NULL REFERENCES scan_jobs(id),
    path        TEXT    NOT NULL,
    error       TEXT    NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_media_album_id ON media(album_id);
CREATE INDEX IF NOT EXISTS idx_albums_parent  ON albums(parent_album_id);

`

// alterations are run after schema creation; "duplicate column" errors are ignored.
var alterations = []string{
	`ALTER TABLE albums ADD COLUMN dir_mtime_ns INTEGER`,
	`ALTER TABLE media ADD COLUMN thumb_ready INTEGER NOT NULL DEFAULT 0`,
	`CREATE TABLE IF NOT EXISTS transcode_jobs (
		id          TEXT    PRIMARY KEY,
		media_id    INTEGER NOT NULL REFERENCES media(id),
		status      TEXT    NOT NULL CHECK(status IN ('queued','running','success','failed')),
		output_path TEXT,
		error       TEXT,
		created_at  TEXT    NOT NULL,
		finished_at TEXT
	)`,
	`CREATE INDEX IF NOT EXISTS idx_transcode_jobs_media_id ON transcode_jobs(media_id)`,
	// Extend scan_jobs.mode to allow 'orphan'.
	// SQLite cannot ALTER a CHECK constraint, so recreate the table.
	`CREATE TABLE IF NOT EXISTS scan_jobs_new (
		id              TEXT    PRIMARY KEY,
		mode            TEXT    NOT NULL CHECK(mode IN ('full','quick','orphan')),
		status          TEXT    NOT NULL CHECK(status IN ('queued','running','success','failed')),
		started_at      TEXT,
		finished_at     TEXT,
		scanned_files   INTEGER NOT NULL DEFAULT 0,
		indexed_files   INTEGER NOT NULL DEFAULT 0,
		skipped_files   INTEGER NOT NULL DEFAULT 0,
		error_count     INTEGER NOT NULL DEFAULT 0,
		fallback_to_full INTEGER NOT NULL DEFAULT 0
	)`,
	`INSERT OR IGNORE INTO scan_jobs_new SELECT * FROM scan_jobs`,
	`DROP TABLE scan_jobs`,
	`ALTER TABLE scan_jobs_new RENAME TO scan_jobs`,
}

func Migrate(s *Store) error {
	if _, err := s.db.Exec(schema); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	for _, stmt := range alterations {
		if _, err := s.db.Exec(stmt); err != nil {
			// SQLite error text for duplicate column: "duplicate column name"
			if !strings.Contains(err.Error(), "duplicate column") {
				return fmt.Errorf("alter: %w", err)
			}
		}
	}
	return nil
}
