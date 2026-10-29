package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func openCampaignStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "data", "reelanvil.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	return s
}

func TestMigrateVersionOnePreservesStorageProbe(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`CREATE TABLE storage_probe(id INTEGER PRIMARY KEY CHECK(id=1),value TEXT NOT NULL); INSERT INTO storage_probe VALUES(1,'existing'); PRAGMA user_version=1`); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var version int
	var value string
	if err = s.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if err = s.db.QueryRow(`SELECT value FROM storage_probe WHERE id=1`).Scan(&value); err != nil {
		t.Fatal(err)
	}
	if version != 2 || value != "existing" {
		t.Fatalf("migration result version=%d value=%q", version, value)
	}
	var tableCount int
	if err = s.db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name IN ('campaign_sources','tracked_campaigns')`).Scan(&tableCount); err != nil {
		t.Fatal(err)
	}
	if tableCount != 2 {
		t.Fatalf("campaign tables = %d, want 2", tableCount)
	}
}

func TestManualCampaignValidationAndDeduplication(t *testing.T) {
	s := openCampaignStore(t)
	ctx := context.Background()
	payout := int64(4500)
	in := CampaignInput{Platform: "vyro", URL: "HTTPS://Example.com:443/campaign?id=3&utm_source=feed#overview", Title: "Sample <Campaign>", Requirements: "One short video", PayoutCents: &payout, Notes: "Keep this note"}
	c, created, err := s.AddManualCampaign(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if !created || !c.Tracked || c.UserStatus != "Interested" || c.Notes != "Keep this note" {
		t.Fatalf("unexpected created campaign: %+v created=%v", c, created)
	}
	if c.URL != "https://example.com/campaign?id=3" {
		t.Fatalf("canonical URL=%q", c.URL)
	}
	dup, created, err := s.AddManualCampaign(ctx, CampaignInput{Platform: "vyro", URL: "https://example.com/campaign?id=3&utm_campaign=other", Title: "Different"})
	if err != nil {
		t.Fatal(err)
	}
	if created || dup.ID != c.ID || dup.Title != c.Title {
		t.Fatalf("duplicate overwrote original: %+v created=%v", dup, created)
	}
	if err = s.TrackCampaign(ctx, c.ID, "Active", "Preserved note"); err != nil {
		t.Fatal(err)
	}
	_, err = s.UpsertDiscoveredCampaign(ctx, Campaign{Platform: "vyro", ExternalID: "external-3", URL: "https://example.com/campaign?id=3", Title: "Refreshed title", Requirements: "Updated requirements", Provenance: "Public source", PlatformStatus: "active"})
	if err != nil {
		t.Fatal(err)
	}
	refreshed, err := s.CampaignByID(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.Title != "Refreshed title" || refreshed.UserStatus != "Active" || refreshed.Notes != "Preserved note" {
		t.Fatalf("discovery overwrote local tracking: %+v", refreshed)
	}
}

func TestCampaignInputsRejectUnsafeURLsAndInvalidFields(t *testing.T) {
	s := openCampaignStore(t)
	ctx := context.Background()
	for _, tc := range []struct {
		name  string
		input CampaignInput
	}{
		{"http", CampaignInput{Platform: "vyro", URL: "http://example.com/", Title: "x"}},
		{"userinfo", CampaignInput{Platform: "vyro", URL: "https://user:pass@example.com/", Title: "x"}},
		{"javascript external", CampaignInput{Platform: "vyro", URL: "https://example.com/", Title: "x", ApplyURL: "javascript:alert(1)"}},
		{"unknown platform", CampaignInput{Platform: "other", URL: "https://example.com/", Title: "x"}},
		{"empty title", CampaignInput{Platform: "vyro", URL: "https://example.com/", Title: " "}},
		{"negative amount", CampaignInput{Platform: "vyro", URL: "https://example.com/", Title: "x", BudgetCents: ptr(int64(-1))}},
		{"bad date", CampaignInput{Platform: "vyro", URL: "https://example.com/", Title: "x", Deadline: "soon"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := s.AddManualCampaign(ctx, tc.input); err == nil {
				t.Fatal("invalid campaign input accepted")
			}
		})
	}
}

func TestCampaignURLIPv6Canonicalization(t *testing.T) {
	got, err := CanonicalCampaignURL("https://[2001:db8::1]:443/campaign")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://[2001:db8::1]/campaign" {
		t.Fatalf("canonical IPv6 URL = %q", got)
	}
}

func TestDiscoveredCampaignStableIDCanRefreshChangedURL(t *testing.T) {
	s := openCampaignStore(t)
	ctx := context.Background()
	base := Campaign{Platform: "vyro", ExternalID: "stable-url", URL: "https://example.com/old", Title: "Before", Provenance: "public"}
	id, err := s.UpsertDiscoveredCampaign(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	base.URL = "https://example.com/new"
	base.Title = "After"
	id2, err := s.UpsertDiscoveredCampaign(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	if id2 != id {
		t.Fatalf("stable external ID created a second source: %q != %q", id2, id)
	}
	updated, err := s.CampaignByID(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if updated.URL != base.URL || updated.Title != base.Title {
		t.Fatalf("stable source was not refreshed: %+v", updated)
	}
}

func TestUntrackAndCascade(t *testing.T) {
	s := openCampaignStore(t)
	ctx := context.Background()
	c, _, err := s.AddManualCampaign(ctx, CampaignInput{Platform: "content-rewards", URL: "https://example.com/campaign", Title: "A"})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.TrackCampaign(ctx, c.ID, "Active", "Keep this note"); err != nil {
		t.Fatal(err)
	}
	if err = s.UntrackCampaign(ctx, c.ID); err != nil {
		t.Fatal(err)
	}
	c, err = s.CampaignByID(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if c.Tracked {
		t.Fatal("untracked campaign still tracked")
	}
	if c.UserStatus != "Active" || c.Notes != "Keep this note" {
		t.Fatalf("untracking should preserve local status and notes: %+v", c)
	}
	if err = s.TrackCampaign(ctx, c.ID, "", ""); err != nil {
		t.Fatal(err)
	}
	c, err = s.CampaignByID(ctx, c.ID)
	if err != nil || !c.Tracked {
		t.Fatalf("re-track did not restore local tracking: %+v err=%v", c, err)
	}
	if c.UserStatus != "Active" || c.Notes != "Keep this note" {
		t.Fatalf("re-track reset local metadata: %+v", c)
	}
	if err = s.TrackCampaign(ctx, c.ID, "Interested", ""); err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.ExecContext(ctx, `DELETE FROM campaign_sources WHERE id=?`, c.ID); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = s.db.QueryRowContext(ctx, `SELECT count(*) FROM tracked_campaigns WHERE campaign_id=?`, c.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("tracked orphan remained after source deletion: %d", count)
	}
}

func TestEditUntrackedManualSourceDoesNotRetrackOrLoseLocalMetadata(t *testing.T) {
	s := openCampaignStore(t)
	ctx := context.Background()
	c, _, err := s.AddManualCampaign(ctx, CampaignInput{Platform: "vyro", URL: "https://example.com/edit", Title: "Before", Notes: "Private local note"})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.TrackCampaign(ctx, c.ID, "Applied", "Updated private note"); err != nil {
		t.Fatal(err)
	}
	if err = s.UntrackCampaign(ctx, c.ID); err != nil {
		t.Fatal(err)
	}
	if err = s.UpdateManualCampaign(ctx, c.ID, CampaignInput{Platform: "vyro", URL: c.URL, Title: "After"}, "", "", false); err != nil {
		t.Fatal(err)
	}
	c, err = s.CampaignByID(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if c.Tracked || c.Title != "After" || c.UserStatus != "Applied" || c.Notes != "Updated private note" {
		t.Fatalf("editing available source changed tracking metadata: %+v", c)
	}
}

func ptr(v int64) *int64 { return &v }
