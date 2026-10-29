package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

var ErrCampaignNotFound = errors.New("campaign not found")
var ErrCampaignDuplicate = errors.New("campaign already exists")

type Campaign struct {
	ID, Platform, ExternalID, URL, Title, Requirements string
	PayoutCents, BudgetCents                           *int64
	Deadline, PlatformStatus, SourceKind, Provenance   string
	ViewURL, ApplyURL, SubmitURL, VerifiedAt           string
	UserStatus, Notes                                  string
	Tracked                                            bool
	TrackedAt                                          string
}

type CampaignInput struct {
	Platform, URL, Title, Requirements string
	PayoutCents, BudgetCents           *int64
	Deadline, Notes                    string
	ViewURL, ApplyURL, SubmitURL       string
}

func newID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// CanonicalCampaignURL accepts only public HTTPS URLs and removes fragments
// and common tracking parameters before uniqueness checks.
func CanonicalCampaignURL(raw string) (string, error) {
	if len(raw) > 2048 || strings.TrimSpace(raw) != raw || raw == "" {
		return "", errors.New("campaign URL must be between 1 and 2048 characters")
	}
	u, err := url.Parse(raw)
	if err != nil || !strings.EqualFold(u.Scheme, "https") || u.Hostname() == "" || u.User != nil {
		return "", errors.New("campaign URL must be a valid HTTPS URL without user information")
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if net.ParseIP(host) == nil {
		for _, label := range strings.Split(host, ".") {
			if label == "" || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
				return "", errors.New("campaign URL has an invalid host")
			}
			for _, c := range label {
				if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
					return "", errors.New("campaign URL has an invalid host")
				}
			}
		}
	}
	port := u.Port()
	if port != "" && port != "443" {
		return "", errors.New("campaign URL must use the standard HTTPS port")
	}
	if ip := net.ParseIP(host); ip != nil && strings.Contains(host, ":") {
		u.Host = "[" + host + "]"
	} else {
		u.Host = host
	}
	u.Scheme, u.Fragment, u.RawFragment = "https", "", ""
	u.ForceQuery = false
	if u.Path == "" {
		u.Path = "/"
	}
	query := u.Query()
	for key := range query {
		lower := strings.ToLower(key)
		if strings.HasPrefix(lower, "utm_") || lower == "fbclid" || lower == "gclid" {
			query.Del(key)
		}
	}
	u.RawQuery = query.Encode()
	return u.String(), nil
}

func canonicalOptionalURL(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", nil
	}
	return CanonicalCampaignURL(strings.TrimSpace(raw))
}

func validateCampaignInput(in CampaignInput) (CampaignInput, string, error) {
	in.Platform = strings.TrimSpace(strings.ToLower(in.Platform))
	if in.Platform != "content-rewards" && in.Platform != "vyro" {
		return in, "", errors.New("select a supported campaign platform")
	}
	canonical, err := CanonicalCampaignURL(in.URL)
	if err != nil {
		return in, "", err
	}
	in.URL = canonical
	in.Title = strings.TrimSpace(in.Title)
	in.Requirements = strings.TrimSpace(in.Requirements)
	in.Notes = strings.TrimSpace(in.Notes)
	if in.Title == "" || len([]rune(in.Title)) > 160 || strings.ContainsAny(in.Title, "\r\n\x00") {
		return in, "", errors.New("title must be 1-160 characters on one line")
	}
	if len(in.Requirements) > 8000 || len(in.Notes) > 8000 || strings.ContainsRune(in.Requirements, '\x00') || strings.ContainsRune(in.Notes, '\x00') {
		return in, "", errors.New("requirements and notes must be 8000 characters or fewer")
	}
	for _, amount := range []*int64{in.PayoutCents, in.BudgetCents} {
		if amount != nil && (*amount < 0 || *amount > 9_000_000_000_000_000) {
			return in, "", errors.New("amount must be non-negative and within the supported range")
		}
	}
	in.Deadline = strings.TrimSpace(in.Deadline)
	if in.Deadline != "" {
		if t, err := time.Parse("2006-01-02", in.Deadline); err == nil {
			in.Deadline = t.UTC().Format("2006-01-02")
		} else if t, err = time.Parse(time.RFC3339, in.Deadline); err == nil {
			in.Deadline = t.UTC().Format(time.RFC3339)
		} else {
			return in, "", errors.New("deadline must be a calendar date or RFC3339 timestamp")
		}
	}
	for _, target := range []*string{&in.ViewURL, &in.ApplyURL, &in.SubmitURL} {
		*target, err = canonicalOptionalURL(*target)
		if err != nil {
			return in, "", fmt.Errorf("external action URL: %w", err)
		}
	}
	return in, canonical, nil
}

func (s *Store) AddManualCampaign(ctx context.Context, input CampaignInput) (Campaign, bool, error) {
	in, canonical, err := validateCampaignInput(input)
	if err != nil {
		return Campaign{}, false, err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	id, err := newID()
	if err != nil {
		return Campaign{}, false, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Campaign{}, false, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `INSERT INTO campaign_sources
		(id,platform,canonical_url,title,requirements,payout_cents,budget_cents,deadline_utc,source_kind,provenance,view_url,apply_url,submit_url,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?, 'manual','Manually added by user',?,?,?,?,?) ON CONFLICT(platform,canonical_url) DO NOTHING`,
		id, in.Platform, canonical, in.Title, in.Requirements, in.PayoutCents, in.BudgetCents, nullableString(in.Deadline), in.URL, in.ViewURL, in.ApplyURL, in.SubmitURL, now, now)
	if err != nil {
		return Campaign{}, false, err
	}
	created, err := result.RowsAffected()
	if err != nil {
		return Campaign{}, false, err
	}
	if created == 0 {
		c, err := scanCampaign(tx.QueryRowContext(ctx, campaignSelect+` WHERE c.platform=? AND c.canonical_url=?`, in.Platform, canonical))
		return c, false, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO tracked_campaigns(campaign_id,user_status,notes,is_tracked,tracked_at,updated_at) VALUES(?,'Interested',?,1,?,?)`, id, strings.TrimSpace(input.Notes), now, now); err != nil {
		return Campaign{}, false, err
	}
	if err = tx.Commit(); err != nil {
		return Campaign{}, false, err
	}
	c, err := s.CampaignByID(ctx, id)
	return c, true, err
}

func nullableString(v string) any {
	if v == "" {
		return nil
	}
	return v
}

func (s *Store) CampaignByURL(ctx context.Context, platform, canonical string) (Campaign, error) {
	return scanCampaign(s.db.QueryRowContext(ctx, campaignSelect+` WHERE c.platform=? AND c.canonical_url=?`, platform, canonical))
}
func (s *Store) CampaignByID(ctx context.Context, id string) (Campaign, error) {
	c, err := scanCampaign(s.db.QueryRowContext(ctx, campaignSelect+` WHERE c.id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Campaign{}, ErrCampaignNotFound
	}
	return c, err
}

const campaignSelect = `SELECT c.id,c.platform,COALESCE(c.external_id,''),c.canonical_url,c.title,c.requirements,c.payout_cents,c.budget_cents,COALESCE(c.deadline_utc,''),c.platform_status,c.source_kind,c.provenance,c.view_url,c.apply_url,c.submit_url,COALESCE(c.verified_at,''),COALESCE(t.user_status,''),COALESCE(t.notes,''),COALESCE(t.is_tracked,0)=1,COALESCE(t.tracked_at,'') FROM campaign_sources c LEFT JOIN tracked_campaigns t ON t.campaign_id=c.id`

func scanCampaign(row *sql.Row) (Campaign, error) {
	var c Campaign
	var payout, budget sql.NullInt64
	err := row.Scan(&c.ID, &c.Platform, &c.ExternalID, &c.URL, &c.Title, &c.Requirements, &payout, &budget, &c.Deadline, &c.PlatformStatus, &c.SourceKind, &c.Provenance, &c.ViewURL, &c.ApplyURL, &c.SubmitURL, &c.VerifiedAt, &c.UserStatus, &c.Notes, &c.Tracked, &c.TrackedAt)
	if err != nil {
		return Campaign{}, err
	}
	if payout.Valid {
		c.PayoutCents = &payout.Int64
	}
	if budget.Valid {
		c.BudgetCents = &budget.Int64
	}
	return c, nil
}

func (s *Store) ListCampaigns(ctx context.Context) ([]Campaign, error) {
	rows, err := s.db.QueryContext(ctx, campaignSelect+` ORDER BY lower(c.title),c.platform`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var campaigns []Campaign
	for rows.Next() {
		var c Campaign
		var payout, budget sql.NullInt64
		if err := rows.Scan(&c.ID, &c.Platform, &c.ExternalID, &c.URL, &c.Title, &c.Requirements, &payout, &budget, &c.Deadline, &c.PlatformStatus, &c.SourceKind, &c.Provenance, &c.ViewURL, &c.ApplyURL, &c.SubmitURL, &c.VerifiedAt, &c.UserStatus, &c.Notes, &c.Tracked, &c.TrackedAt); err != nil {
			return nil, err
		}
		if payout.Valid {
			c.PayoutCents = &payout.Int64
		}
		if budget.Valid {
			c.BudgetCents = &budget.Int64
		}
		campaigns = append(campaigns, c)
	}
	return campaigns, rows.Err()
}

func (s *Store) TrackCampaign(ctx context.Context, id, status, notes string) error {
	status = strings.TrimSpace(status)
	if status != "" && !validCampaignStatus(status) {
		return errors.New("invalid tracked campaign status")
	}
	if len(notes) > 8000 || strings.ContainsRune(notes, '\x00') {
		return errors.New("notes must be 8000 characters or fewer")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if status == "" {
		var existing string
		err := s.db.QueryRowContext(ctx, `SELECT user_status FROM tracked_campaigns WHERE campaign_id=?`, id).Scan(&existing)
		if errors.Is(err, sql.ErrNoRows) {
			status = "Interested"
		} else if err != nil {
			return err
		} else {
			status = existing
		}
	}
	result, err := s.db.ExecContext(ctx, `INSERT INTO tracked_campaigns(campaign_id,user_status,notes,is_tracked,tracked_at,updated_at)
		SELECT id,?,?,?,?,? FROM campaign_sources WHERE id=?
		ON CONFLICT(campaign_id) DO UPDATE SET user_status=excluded.user_status,notes=CASE WHEN excluded.notes='' THEN tracked_campaigns.notes ELSE excluded.notes END,is_tracked=1,updated_at=excluded.updated_at`, status, strings.TrimSpace(notes), 1, now, now, id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err == nil && n == 0 {
		return ErrCampaignNotFound
	}
	return err
}
func validCampaignStatus(status string) bool {
	switch status {
	case "Interested", "Applied", "Active", "Completed", "Archived":
		return true
	}
	return false
}
func (s *Store) UntrackCampaign(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE tracked_campaigns SET is_tracked=0,updated_at=? WHERE campaign_id=?`, time.Now().UTC().Format(time.RFC3339Nano), id)
	return err
}

func (s *Store) UpdateManualCampaign(ctx context.Context, id string, input CampaignInput, status, notes string, tracked bool) error {
	in, canonical, err := validateCampaignInput(input)
	if err != nil {
		return err
	}
	if tracked && !validCampaignStatus(status) {
		return errors.New("invalid tracked campaign status")
	}
	if tracked && (len(notes) > 8000 || strings.ContainsRune(notes, '\x00')) {
		return errors.New("notes must be 8000 characters or fewer")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE campaign_sources SET platform=?,canonical_url=?,title=?,requirements=?,payout_cents=?,budget_cents=?,deadline_utc=?,view_url=?,apply_url=?,submit_url=?,updated_at=? WHERE id=? AND source_kind='manual'`, in.Platform, canonical, in.Title, in.Requirements, in.PayoutCents, in.BudgetCents, nullableString(in.Deadline), in.ViewURL, in.ApplyURL, in.SubmitURL, now, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrCampaignNotFound
	}
	if tracked {
		if _, err = tx.ExecContext(ctx, `INSERT INTO tracked_campaigns(campaign_id,user_status,notes,is_tracked,tracked_at,updated_at) VALUES(?,?,?,1,?,?) ON CONFLICT(campaign_id) DO UPDATE SET user_status=excluded.user_status,notes=excluded.notes,is_tracked=1,updated_at=excluded.updated_at`, id, status, strings.TrimSpace(notes), now, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// UpsertDiscoveredCampaign updates only platform-owned fields. The tracking
// row intentionally remains untouched so discovery refreshes preserve notes/status.
func (s *Store) UpsertDiscoveredCampaign(ctx context.Context, c Campaign) (string, error) {
	input, canonical, err := validateCampaignInput(CampaignInput{Platform: c.Platform, URL: c.URL, Title: c.Title, Requirements: c.Requirements, PayoutCents: c.PayoutCents, BudgetCents: c.BudgetCents, Deadline: c.Deadline, ViewURL: c.ViewURL, ApplyURL: c.ApplyURL, SubmitURL: c.SubmitURL})
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(c.ExternalID) == "" || len(c.ExternalID) > 256 {
		return "", errors.New("discovered campaign needs a stable external ID")
	}
	id, err := newID()
	if err != nil {
		return "", err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	const updateFields = `canonical_url=?,title=?,requirements=?,payout_cents=?,budget_cents=?,deadline_utc=?,platform_status=?,provenance=?,view_url=?,apply_url=?,submit_url=?,verified_at=?,updated_at=?`
	var existingID string
	err = tx.QueryRowContext(ctx, `SELECT id FROM campaign_sources WHERE platform=? AND external_id=?`, input.Platform, c.ExternalID).Scan(&existingID)
	if err == nil {
		_, err = tx.ExecContext(ctx, `UPDATE campaign_sources SET `+updateFields+` WHERE id=?`, canonical, input.Title, input.Requirements, input.PayoutCents, input.BudgetCents, nullableString(input.Deadline), strings.TrimSpace(c.PlatformStatus), strings.TrimSpace(c.Provenance), input.ViewURL, input.ApplyURL, input.SubmitURL, now, now, existingID)
		if err != nil {
			return "", err
		}
		if err = tx.Commit(); err != nil {
			return "", err
		}
		return existingID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO campaign_sources(id,platform,external_id,canonical_url,title,requirements,payout_cents,budget_cents,deadline_utc,platform_status,source_kind,provenance,view_url,apply_url,submit_url,verified_at,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,'discovered',?,?,?,?,?,?,?)
	ON CONFLICT(platform,canonical_url) DO UPDATE SET external_id=excluded.external_id,title=excluded.title,requirements=excluded.requirements,payout_cents=excluded.payout_cents,budget_cents=excluded.budget_cents,deadline_utc=excluded.deadline_utc,platform_status=excluded.platform_status,provenance=excluded.provenance,view_url=excluded.view_url,apply_url=excluded.apply_url,submit_url=excluded.submit_url,verified_at=excluded.verified_at,updated_at=excluded.updated_at`, id, input.Platform, c.ExternalID, canonical, input.Title, input.Requirements, input.PayoutCents, input.BudgetCents, nullableString(input.Deadline), strings.TrimSpace(c.PlatformStatus), strings.TrimSpace(c.Provenance), input.ViewURL, input.ApplyURL, input.SubmitURL, now, now, now)
	if err != nil {
		return "", err
	}
	if err = tx.QueryRowContext(ctx, `SELECT id FROM campaign_sources WHERE platform=? AND canonical_url=?`, input.Platform, canonical).Scan(&existingID); err != nil {
		return "", err
	}
	if err = tx.Commit(); err != nil {
		return "", err
	}
	return existingID, nil
}

func ParseMoneyCents(value string) (*int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	if strings.HasPrefix(value, "-") || strings.HasPrefix(value, "+") {
		return nil, errors.New("amount must be non-negative")
	}
	parts := strings.Split(value, ".")
	if len(parts) > 2 || parts[0] == "" || len(parts) == 2 && len(parts[1]) > 2 {
		return nil, errors.New("amount must have at most two decimal places")
	}
	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || whole > 90_000_000_000_000 {
		return nil, errors.New("amount is out of range")
	}
	fraction := "00"
	if len(parts) == 2 {
		fraction = parts[1] + strings.Repeat("0", 2-len(parts[1]))
	}
	cents, err := strconv.ParseInt(fraction, 10, 64)
	if err != nil {
		return nil, errors.New("invalid amount")
	}
	valueCents := whole*100 + cents
	return &valueCents, nil
}
func FormatMoney(cents *int64) string {
	if cents == nil {
		return "Unknown"
	}
	return fmt.Sprintf("$%d.%02d", *cents/100, *cents%100)
}
func CanonicalDigest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

// StableSortCampaigns gives deterministic in-memory sorting for supported UI filters.
func StableSortCampaigns(campaigns []Campaign, less func(a, b Campaign) bool) {
	sort.SliceStable(campaigns, func(i, j int) bool { return less(campaigns[i], campaigns[j]) })
}
