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

// aiTables are created once; IF NOT EXISTS makes them idempotent.
var aiTables = []string{
	`CREATE TABLE IF NOT EXISTS ai_jobs (
		media_id     INTEGER NOT NULL PRIMARY KEY REFERENCES media(id),
		status       TEXT    NOT NULL CHECK(status IN ('queued','running','success','failed')),
		attempts     INTEGER NOT NULL DEFAULT 0,
		queued_at    TEXT    NOT NULL,
		finished_at  TEXT,
		error        TEXT
	)`,
	`CREATE TABLE IF NOT EXISTS ai_labels (
		id           INTEGER PRIMARY KEY AUTOINCREMENT,
		media_id     INTEGER NOT NULL REFERENCES media(id),
		label        TEXT    NOT NULL,
		confidence   REAL    NOT NULL,
		source       TEXT    NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS idx_ai_labels_media ON ai_labels(media_id)`,
	`CREATE TABLE IF NOT EXISTS ai_faces (
		id           INTEGER PRIMARY KEY AUTOINCREMENT,
		media_id     INTEGER NOT NULL REFERENCES media(id),
		bbox_json    TEXT    NOT NULL,
		embedding    BLOB    NOT NULL,
		crop_path    TEXT    NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS idx_ai_faces_media ON ai_faces(media_id)`,
}

// simpleAlterations are run after schema creation; "duplicate column" errors are ignored.
var simpleAlterations = []string{
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
}

// scanJobsRecreation extends scan_jobs.mode to allow 'orphan'.
// SQLite cannot ALTER a CHECK constraint, so the table is recreated.
// These three statements must run atomically.
var scanJobsRecreation = []string{
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
	for _, stmt := range simpleAlterations {
		if _, err := s.db.Exec(stmt); err != nil {
			if !strings.Contains(err.Error(), "duplicate column") {
				return fmt.Errorf("alter: %w", err)
			}
		}
	}
	if err := migrateScanJobsAtomic(s); err != nil {
		return err
	}
	for _, stmt := range aiTables {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("ai table migration: %w", err)
		}
	}
	return nil
}

// migrateScanJobsAtomic runs the scan_jobs table recreation in a single transaction
// so a crash between DROP and RENAME cannot leave the database without the table.
func migrateScanJobsAtomic(s *Store) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("migration tx begin: %w", err)
	}
	for _, stmt := range scanJobsRecreation {
		if _, err := tx.Exec(stmt); err != nil {
			_ = tx.Rollback()
			// "already exists" means this migration already completed successfully.
			if strings.Contains(err.Error(), "already exists") || strings.Contains(err.Error(), "no such table") {
				return nil
			}
			return fmt.Errorf("migration tx: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("migration tx commit: %w", err)
	}
	return nil
}
