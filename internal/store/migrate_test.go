package store

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
)

// buildV2Database creates a schema-v2 database the way Build 0.9 did,
// including a manual row written by the shifted-column insert bug.
func buildV2Database(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "v2.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = migrateStorageProbeV1(ctx, tx); err != nil {
		t.Fatal(err)
	}
	if err = migrateCampaignsV2(ctx, tx); err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		// Shifted manual row: view_url holds the campaign URL, the real view
		// URL sits in apply_url, apply in submit_url, submit in created_at.
		`INSERT INTO campaign_sources(id,platform,canonical_url,title,requirements,deadline_utc,source_kind,provenance,view_url,apply_url,submit_url,created_at,updated_at)
			VALUES('shifted','vyro','https://example.com/a','Shifted','', '2026-11-02','manual','Manually added by user','https://example.com/a','https://example.com/view','https://example.com/apply','https://example.com/submit','2026-10-01T10:00:00Z')`,
		// Shifted row with no optional URLs, later edited: only created_at is wrong.
		`INSERT INTO campaign_sources(id,platform,canonical_url,title,requirements,source_kind,provenance,view_url,apply_url,submit_url,created_at,updated_at)
			VALUES('edited','content-rewards','https://example.com/b','Edited','', 'manual','Manually added by user','','','','','2026-10-02T10:00:00Z')`,
		`INSERT INTO campaign_sources(id,platform,external_id,canonical_url,title,requirements,deadline_utc,source_kind,provenance,created_at,updated_at)
			VALUES('found','vyro','ext-1','https://example.com/c','Discovered','', '2026-11-03T18:30:00Z','discovered','public','2026-10-03T10:00:00Z','2026-10-03T10:00:00Z')`,
		`INSERT INTO tracked_campaigns VALUES('shifted','Active','keep me',1,'2026-10-01T10:00:00Z','2026-10-01T10:00:00Z')`,
		`PRAGMA user_version=2`,
	} {
		if _, err = tx.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("%s: %v", stmt, err)
		}
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestMigrateV2ToV3RepairsShiftedRowsAndNormalizesDeadlines(t *testing.T) {
	s, err := Open(buildV2Database(t))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	shifted, err := s.CampaignByID(ctx, "shifted")
	if err != nil {
		t.Fatal(err)
	}
	if shifted.ViewURL != "https://example.com/view" || shifted.ApplyURL != "https://example.com/apply" || shifted.SubmitURL != "https://example.com/submit" {
		t.Fatalf("shifted URLs not repaired: %+v", shifted)
	}
	if !shifted.Tracked || shifted.UserStatus != "Active" || shifted.Notes != "keep me" || shifted.Deadline != "2026-11-02" {
		t.Fatalf("tracking data lost in migration: %+v", shifted)
	}
	found, err := s.CampaignByID(ctx, "found")
	if err != nil || found.Deadline != "2026-11-03" {
		t.Fatalf("timestamp deadline not normalized to a date: %+v err=%v", found, err)
	}
	var created string
	for _, id := range []string{"shifted", "edited"} {
		if err = s.db.QueryRow(`SELECT created_at FROM campaign_sources WHERE id=?`, id).Scan(&created); err != nil || created == "" {
			t.Fatalf("%s created_at = %q err=%v", id, created, err)
		}
	}
	edited, err := s.CampaignByID(ctx, "edited")
	if err != nil || edited.ViewURL != "" || edited.ApplyURL != "" {
		t.Fatalf("unshifted row changed: %+v err=%v", edited, err)
	}
	// Cascade still works after the table rebuild.
	if _, err = s.db.Exec(`DELETE FROM campaign_sources WHERE id='shifted'`); err != nil {
		t.Fatal(err)
	}
	var orphans int
	if err = s.db.QueryRow(`SELECT count(*) FROM tracked_campaigns`).Scan(&orphans); err != nil || orphans != 0 {
		t.Fatalf("tracked rows after cascade = %d err=%v", orphans, err)
	}
}

func TestManualCampaignStoresActionURLsInTheirColumns(t *testing.T) {
	s := openCampaignStore(t)
	ctx := context.Background()
	c, created, err := s.AddManualCampaign(ctx, CampaignInput{Platform: "vyro", URL: "https://example.com/main", Title: "URLs", ViewURL: "https://example.com/view", ApplyURL: "https://example.com/apply", SubmitURL: "https://example.com/submit", Deadline: "2026-12-01T05:00:00+02:00"})
	if err != nil || !created {
		t.Fatalf("created=%v err=%v", created, err)
	}
	if c.ViewURL != "https://example.com/view" || c.ApplyURL != "https://example.com/apply" || c.SubmitURL != "https://example.com/submit" {
		t.Fatalf("action URLs stored in the wrong columns: %+v", c)
	}
	if c.Deadline != "2026-12-01" {
		t.Fatalf("deadline = %q, want UTC calendar date", c.Deadline)
	}
	var createdAt string
	if err = s.db.QueryRow(`SELECT created_at FROM campaign_sources WHERE id=?`, c.ID).Scan(&createdAt); err != nil || createdAt == "" {
		t.Fatalf("created_at = %q err=%v", createdAt, err)
	}
}

func TestMissingCampaignAndDuplicateURLErrors(t *testing.T) {
	s := openCampaignStore(t)
	ctx := context.Background()
	if err := s.UntrackCampaign(ctx, "missing"); !errors.Is(err, ErrCampaignNotFound) {
		t.Fatalf("untrack missing = %v", err)
	}
	if err := s.TrackCampaign(ctx, "missing", "", ""); !errors.Is(err, ErrCampaignNotFound) {
		t.Fatalf("track missing = %v", err)
	}
	a, _, err := s.AddManualCampaign(ctx, CampaignInput{Platform: "vyro", URL: "https://example.com/a", Title: "A"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.AddManualCampaign(ctx, CampaignInput{Platform: "vyro", URL: "https://example.com/b", Title: "B"}); err != nil {
		t.Fatal(err)
	}
	err = s.UpdateManualCampaign(ctx, a.ID, CampaignInput{Platform: "vyro", URL: "https://example.com/b", Title: "A"}, "Interested", "", true)
	if !errors.Is(err, ErrCampaignDuplicate) {
		t.Fatalf("duplicate URL update = %v, want ErrCampaignDuplicate", err)
	}
	var input InputError
	if _, _, err = s.AddManualCampaign(ctx, CampaignInput{Platform: "other", URL: "https://example.com/c", Title: "C"}); !errors.As(err, &input) {
		t.Fatalf("validation error is not an InputError: %v", err)
	}
}

func TestParseMoneyCents(t *testing.T) {
	for in, want := range map[string]int64{"0": 0, "12": 1200, "12.5": 1250, "12.05": 1205, "1.": 100} {
		got, err := ParseMoneyCents(in)
		if err != nil || got == nil || *got != want {
			t.Errorf("ParseMoneyCents(%q) = %v, %v; want %d", in, got, err, want)
		}
	}
	for _, in := range []string{"-1", "+1", "1.-5", "1.+5", "1.234", "abc", ".5", "1.2.3"} {
		if _, err := ParseMoneyCents(in); err == nil {
			t.Errorf("ParseMoneyCents(%q) accepted", in)
		}
	}
}
