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

// migration upgrades the schema from version index to index+1. Migrations run
// in order inside one transaction, so a failure leaves user_version unchanged.
type migration func(context.Context, *sql.Tx) error

var migrations = []migration{
	migrateStorageProbeV1,
	migrateCampaignsV2,
	migrateCampaignsV3,
}

// SchemaVersion is the database schema version this build writes.
var SchemaVersion = len(migrations)

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
	if v > SchemaVersion {
		return fmt.Errorf("unsupported database schema version %d", v)
	}
	if v == SchemaVersion {
		return nil
	}
	for ; v < SchemaVersion; v++ {
		if err = migrations[v](ctx, tx); err != nil {
			return fmt.Errorf("schema migration to version %d: %w", v+1, err)
		}
	}
	// Table rebuilds must not leave dangling references behind.
	var violations int
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM pragma_foreign_key_check").Scan(&violations); err != nil {
		return err
	}
	if violations > 0 {
		return fmt.Errorf("schema migration left %d foreign key violations", violations)
	}
	if _, err = tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version=%d", SchemaVersion)); err != nil {
		return err
	}
	return tx.Commit()
}

func migrateStorageProbeV1(ctx context.Context, tx *sql.Tx) error {
	if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS storage_probe (id INTEGER PRIMARY KEY CHECK(id=1), value TEXT NOT NULL)`); err != nil {
		return err
	}
	return validateSchema(ctx, tx)
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
	return execAll(ctx, tx, statements)
}

// migrateCampaignsV3 rebuilds the campaign tables so that:
//   - platform and status lists are validated in Go (Platforms, Statuses)
//     instead of CHECK constraints, which SQLite can only change by rebuilding
//     the table;
//   - deadlines are stored in one format, a YYYY-MM-DD calendar date;
//   - manual records written by the Build 0.9 insert bug are repaired. That
//     insert passed one argument too many, which stored the campaign URL as
//     view_url and shifted the view, apply and submit URLs one column right,
//     leaving the submit URL (usually empty) in created_at.
func migrateCampaignsV3(ctx context.Context, tx *sql.Tx) error {
	statements := []string{
		`UPDATE campaign_sources SET view_url=apply_url, apply_url=submit_url, submit_url=created_at, created_at=updated_at
			WHERE source_kind='manual' AND created_at NOT GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T*' AND view_url=canonical_url`,
		`UPDATE campaign_sources SET created_at=updated_at
			WHERE created_at NOT GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T*'`,
		`UPDATE campaign_sources SET submit_url='' WHERE submit_url<>'' AND submit_url NOT LIKE 'https://%'`,
		`CREATE TABLE campaign_sources_v3 (
			id TEXT PRIMARY KEY,
			platform TEXT NOT NULL CHECK(length(platform) BETWEEN 1 AND 64),
			external_id TEXT,
			canonical_url TEXT NOT NULL,
			title TEXT NOT NULL CHECK(length(title) BETWEEN 1 AND 160),
			requirements TEXT NOT NULL DEFAULT '',
			payout_cents INTEGER CHECK(payout_cents IS NULL OR payout_cents >= 0),
			budget_cents INTEGER CHECK(budget_cents IS NULL OR budget_cents >= 0),
			deadline TEXT CHECK(deadline IS NULL OR deadline GLOB '[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]'),
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
		`INSERT INTO campaign_sources_v3 SELECT id,platform,external_id,canonical_url,title,requirements,payout_cents,budget_cents,
			CASE WHEN deadline_utc IS NULL OR deadline_utc='' THEN NULL ELSE substr(deadline_utc,1,10) END,
			platform_status,source_kind,provenance,view_url,apply_url,submit_url,verified_at,created_at,updated_at FROM campaign_sources`,
		`CREATE TABLE tracked_campaigns_v3 (
			campaign_id TEXT PRIMARY KEY REFERENCES campaign_sources_v3(id) ON DELETE CASCADE,
			user_status TEXT NOT NULL CHECK(length(user_status) BETWEEN 1 AND 32),
			notes TEXT NOT NULL DEFAULT '',
			is_tracked INTEGER NOT NULL DEFAULT 1 CHECK(is_tracked IN (0,1)),
			tracked_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`INSERT INTO tracked_campaigns_v3 SELECT campaign_id,user_status,notes,is_tracked,tracked_at,updated_at FROM tracked_campaigns`,
		`DROP TABLE tracked_campaigns`,
		`DROP TABLE campaign_sources`,
		// Renaming also rewrites tracked_campaigns_v3's foreign key reference.
		`ALTER TABLE campaign_sources_v3 RENAME TO campaign_sources`,
		`ALTER TABLE tracked_campaigns_v3 RENAME TO tracked_campaigns`,
		`CREATE UNIQUE INDEX campaign_sources_external_id ON campaign_sources(platform, external_id) WHERE external_id IS NOT NULL AND external_id <> ''`,
		`CREATE INDEX tracked_campaigns_status ON tracked_campaigns(user_status, tracked_at)`,
	}
	return execAll(ctx, tx, statements)
}

func execAll(ctx context.Context, tx *sql.Tx, statements []string) error {
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return err
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
