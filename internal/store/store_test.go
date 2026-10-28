package store

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

func TestPersistentReadWriteAndReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "reelanvil.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.CheckReadWrite(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.CheckReadWrite(context.Background()); err != nil {
		t.Fatal(err)
	}
	var version int
	if err = s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 1 {
		t.Fatalf("user_version = %d, err = %v; want 1", version, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("database permissions = %o, want 600", info.Mode().Perm())
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		info, err = os.Stat(path + suffix)
		if err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		if err == nil && info.Mode().Perm() != 0600 {
			t.Errorf("database sidecar %s permissions = %o, want 600", suffix, info.Mode().Perm())
		}
	}
}
func TestFutureSchemaRejected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec("PRAGMA user_version=99"); err != nil {
		t.Fatal(err)
	}
	s.Close()
	if _, err = Open(path); err == nil {
		t.Fatal("future schema accepted")
	}
}

func TestInvalidDatabaseDirectoryRejectedWithoutOverwrite(t *testing.T) {
	parent := t.TempDir()
	marker := filepath.Join(parent, "not-a-directory")
	if err := os.WriteFile(marker, []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(filepath.Join(marker, "reelanvil.db")); err == nil {
		t.Fatal("database open succeeded below a regular file")
	}
	got, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "preserve" {
		t.Fatalf("existing file changed: %q", got)
	}
}

func TestInvalidExistingSchemaDoesNotAdvanceVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`CREATE TABLE storage_probe (value TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = Open(path); err == nil {
		t.Fatal("invalid existing schema was accepted")
	}
	db, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var version int
	if err = db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 0 {
		t.Fatalf("invalid schema advanced user_version to %d; want 0", version)
	}
	var columnCount int
	if err = db.QueryRow("SELECT count(*) FROM pragma_table_info('storage_probe')").Scan(&columnCount); err != nil {
		t.Fatal(err)
	}
	if columnCount != 1 {
		t.Fatalf("invalid table changed during rejected migration: got %d columns", columnCount)
	}
}
