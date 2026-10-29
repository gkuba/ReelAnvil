package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db   *sql.DB
	path string
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	if err := os.Chmod(filepath.Dir(path), 0700); err != nil {
		return nil, fmt.Errorf("secure database directory: %w", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, p := range []string{"PRAGMA busy_timeout=5000", "PRAGMA foreign_keys=ON"} {
		if _, err = db.ExecContext(ctx, p); err != nil {
			db.Close()
			return nil, fmt.Errorf("sqlite setup: %w", err)
		}
	}
	var journalMode string
	if err = db.QueryRowContext(ctx, "PRAGMA journal_mode=WAL").Scan(&journalMode); err != nil {
		db.Close()
		return nil, fmt.Errorf("sqlite WAL setup: %w", err)
	}
	if journalMode != "wal" {
		db.Close()
		return nil, fmt.Errorf("sqlite WAL setup: expected wal mode, got %q", journalMode)
	}
	if err = migrate(ctx, db); err != nil {
		db.Close()
		return nil, err
	}
	s := &Store{db: db, path: path}
	if err = s.secureFiles(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}
func migrate(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var v int
	if err = tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&v); err != nil {
		return err
	}
	if v > 2 {
		return fmt.Errorf("unsupported database schema version %d", v)
	}
	if v == 0 {
		if _, err = tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS storage_probe (id INTEGER PRIMARY KEY CHECK(id=1), value TEXT NOT NULL)`); err != nil {
			return err
		}
		if err = validateSchema(ctx, tx); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, "PRAGMA user_version=1"); err != nil {
			return err
		}
		v = 1
	}
	if v == 1 {
		if err = migrateCampaignsV2(ctx, tx); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, "PRAGMA user_version=2"); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func migrateCampaignsV2(ctx context.Context, tx *sql.Tx) error {
	statements := []string{
		`CREATE TABLE campaign_sources (
			id TEXT PRIMARY KEY,
			platform TEXT NOT NULL CHECK(platform IN ('content-rewards','vyro')),
			external_id TEXT,
			canonical_url TEXT NOT NULL,
			title TEXT NOT NULL CHECK(length(title) BETWEEN 1 AND 160),
			requirements TEXT NOT NULL DEFAULT '',
			payout_cents INTEGER CHECK(payout_cents IS NULL OR payout_cents >= 0),
			budget_cents INTEGER CHECK(budget_cents IS NULL OR budget_cents >= 0),
			deadline_utc TEXT,
			platform_status TEXT NOT NULL DEFAULT '',
			source_kind TEXT NOT NULL CHECK(source_kind IN ('discovered','manual')),
			provenance TEXT NOT NULL,
			view_url TEXT NOT NULL DEFAULT '',
			apply_url TEXT NOT NULL DEFAULT '',
			submit_url TEXT NOT NULL DEFAULT '',
			verified_at TEXT,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			UNIQUE(platform, canonical_url)
		)`,
		`CREATE UNIQUE INDEX campaign_sources_external_id ON campaign_sources(platform, external_id) WHERE external_id IS NOT NULL AND external_id <> ''`,
		`CREATE TABLE tracked_campaigns (
			campaign_id TEXT PRIMARY KEY REFERENCES campaign_sources(id) ON DELETE CASCADE,
			user_status TEXT NOT NULL CHECK(user_status IN ('Interested','Applied','Active','Completed','Archived')),
			notes TEXT NOT NULL DEFAULT '',
			is_tracked INTEGER NOT NULL DEFAULT 1 CHECK(is_tracked IN (0,1)),
			tracked_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE INDEX tracked_campaigns_status ON tracked_campaigns(user_status, tracked_at)`,
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("campaign migration: %w", err)
		}
	}
	return nil
}

func validateSchema(ctx context.Context, tx *sql.Tx) error {
	rows, err := tx.QueryContext(ctx, "PRAGMA table_info(storage_probe)")
	if err != nil {
		return err
	}
	defer rows.Close()
	columns := make(map[string]bool, 2)
	columnCount := 0
	for rows.Next() {
		columnCount++
		var cid, notNull, primaryKey int
		var name, columnType string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return err
		}
		switch name {
		case "id":
			columns[name] = columnType == "INTEGER" && primaryKey == 1
		case "value":
			columns[name] = columnType == "TEXT" && notNull == 1
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if !columns["id"] || !columns["value"] || columnCount != 2 {
		return fmt.Errorf("schema migration produced an invalid storage_probe table")
	}
	return nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) secureFiles() error {
	for _, suffix := range []string{"", "-wal", "-shm"} {
		path := s.path + suffix
		if err := os.Chmod(path, 0600); err != nil {
			if suffix != "" && os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("secure database file %s: %w", filepath.Base(path), err)
		}
	}
	return nil
}

func (s *Store) CheckReadWrite(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	token := fmt.Sprintf("probe-%d", time.Now().UnixNano())
	if _, err = tx.ExecContext(ctx, `INSERT INTO storage_probe(id,value) VALUES(1,?) ON CONFLICT(id) DO UPDATE SET value=excluded.value`, token); err != nil {
		return fmt.Errorf("write probe: %w", err)
	}
	var got string
	if err = tx.QueryRowContext(ctx, "SELECT value FROM storage_probe WHERE id=1").Scan(&got); err != nil {
		return fmt.Errorf("read probe: %w", err)
	}
	if got != token {
		return fmt.Errorf("readback mismatch")
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return s.secureFiles()
}
