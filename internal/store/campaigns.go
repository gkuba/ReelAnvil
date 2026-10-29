package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var ErrCampaignNotFound = errors.New("campaign not found")
var ErrCampaignDuplicate = errors.New("another campaign on this platform already uses this URL")

// InputError reports user input that failed validation. Its message is safe
// to show in the UI, unlike database or I/O errors.
type InputError struct{ msg string }

func (e InputError) Error() string { return e.msg }

// InputErrorf returns an InputError with a formatted message.
func InputErrorf(format string, args ...any) error {
	return InputError{fmt.Sprintf(format, args...)}
}

// DateLayout is the only stored deadline format: a calendar date.
const DateLayout = "2006-01-02"

type Platform struct{ ID, Name string }

// Platforms are the campaign platforms ReelAnvil can record. Adding one here
// needs no schema migration.
var Platforms = []Platform{{"content-rewards", "Content Rewards"}, {"vyro", "Vyro"}}

// Statuses are the local tracking statuses, in workflow order.
var Statuses = []string{"Interested", "Applied", "Active", "Completed", "Archived"}

const StatusArchived = "Archived"

func ValidPlatform(id string) bool {
	for _, p := range Platforms {
		if p.ID == id {
			return true
		}
	}
	return false
}

func ValidStatus(status string) bool {
	for _, s := range Statuses {
		if s == status {
			return true
		}
	}
	return false
}

type Campaign struct {
	ID, Platform, ExternalID, URL, Title, Requirements string
	PayoutCents, BudgetCents                           *int64
	Deadline, PlatformStatus, SourceKind, Provenance   string
	ViewURL, ApplyURL, SubmitURL, VerifiedAt           string
	UserStatus, Notes                                  string
	Tracked                                            bool
	TrackedAt                                          string
}

// DeadlineDate parses the stored deadline. ok is false when no deadline is known.
func (c Campaign) DeadlineDate() (d time.Time, ok bool) {
	d, err := time.Parse(DateLayout, c.Deadline)
	return d, err == nil
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
		return "", InputErrorf("campaign URL must be between 1 and 2048 characters")
	}
	u, err := url.Parse(raw)
	if err != nil || !strings.EqualFold(u.Scheme, "https") || u.Hostname() == "" || u.User != nil {
		return "", InputErrorf("campaign URL must be a valid HTTPS URL without user information")
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if net.ParseIP(host) == nil {
		for _, label := range strings.Split(host, ".") {
			if label == "" || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
				return "", InputErrorf("campaign URL has an invalid host")
			}
			for _, c := range label {
				if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
					return "", InputErrorf("campaign URL has an invalid host")
				}
			}
		}
	}
	port := u.Port()
	if port != "" && port != "443" {
		return "", InputErrorf("campaign URL must use the standard HTTPS port")
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

// normalizeDeadline accepts a calendar date or an RFC3339 timestamp and returns
// the calendar date in UTC.
func normalizeDeadline(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if t, err := time.Parse(DateLayout, value); err == nil {
		return t.Format(DateLayout), nil
	}
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t.UTC().Format(DateLayout), nil
	}
	return "", InputErrorf("deadline must be a calendar date or RFC3339 timestamp")
}

func validateNotes(notes string) error {
	if len(notes) > 8000 || strings.ContainsRune(notes, '\x00') {
		return InputErrorf("notes must be 8000 characters or fewer")
	}
	return nil
}

func validateCampaignInput(in CampaignInput) (CampaignInput, error) {
	in.Platform = strings.TrimSpace(strings.ToLower(in.Platform))
	if !ValidPlatform(in.Platform) {
		return in, InputErrorf("select a supported campaign platform")
	}
	canonical, err := CanonicalCampaignURL(in.URL)
	if err != nil {
		return in, err
	}
	in.URL = canonical
	in.Title = strings.TrimSpace(in.Title)
	in.Requirements = strings.TrimSpace(in.Requirements)
	in.Notes = strings.TrimSpace(in.Notes)
	if in.Title == "" || len([]rune(in.Title)) > 160 || strings.ContainsAny(in.Title, "\r\n\x00") {
		return in, InputErrorf("title must be 1-160 characters on one line")
	}
	if len(in.Requirements) > 8000 || strings.ContainsRune(in.Requirements, '\x00') {
		return in, InputErrorf("requirements must be 8000 characters or fewer")
	}
	if err = validateNotes(in.Notes); err != nil {
		return in, err
	}
	for _, amount := range []*int64{in.PayoutCents, in.BudgetCents} {
		if amount != nil && (*amount < 0 || *amount > 9_000_000_000_000_000) {
			return in, InputErrorf("amount must be non-negative and within the supported range")
		}
	}
	if in.Deadline, err = normalizeDeadline(in.Deadline); err != nil {
		return in, err
	}
	for _, target := range []*string{&in.ViewURL, &in.ApplyURL, &in.SubmitURL} {
		*target, err = canonicalOptionalURL(*target)
		if err != nil {
			return in, InputErrorf("external action URL: %v", err)
		}
	}
	return in, nil
}

// isUniqueViolation reports whether err is a SQLite UNIQUE constraint failure.
func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

func now() string { return time.Now().UTC().Format(time.RFC3339Nano) }

func (s *Store) AddManualCampaign(ctx context.Context, input CampaignInput) (Campaign, bool, error) {
	in, err := validateCampaignInput(input)
	if err != nil {
		return Campaign{}, false, err
	}
	ts := now()
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
		(id,platform,canonical_url,title,requirements,payout_cents,budget_cents,deadline,source_kind,provenance,view_url,apply_url,submit_url,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?,'manual','Manually added by user',?,?,?,?,?) ON CONFLICT(platform,canonical_url) DO NOTHING`,
		id, in.Platform, in.URL, in.Title, in.Requirements, in.PayoutCents, in.BudgetCents, nullableString(in.Deadline), in.ViewURL, in.ApplyURL, in.SubmitURL, ts, ts)
	if err != nil {
		return Campaign{}, false, err
	}
	created, err := result.RowsAffected()
	if err != nil {
		return Campaign{}, false, err
	}
	if created == 0 {
		c, err := scanCampaign(tx.QueryRowContext(ctx, campaignSelect+` WHERE c.platform=? AND c.canonical_url=?`, in.Platform, in.URL))
		return c, false, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO tracked_campaigns(campaign_id,user_status,notes,is_tracked,tracked_at,updated_at) VALUES(?,'Interested',?,1,?,?)`, id, in.Notes, ts, ts); err != nil {
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

func (s *Store) CampaignByID(ctx context.Context, id string) (Campaign, error) {
	c, err := scanCampaign(s.db.QueryRowContext(ctx, campaignSelect+` WHERE c.id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Campaign{}, ErrCampaignNotFound
	}
	return c, err
}

const campaignSelect = `SELECT c.id,c.platform,COALESCE(c.external_id,''),c.canonical_url,c.title,c.requirements,c.payout_cents,c.budget_cents,COALESCE(c.deadline,''),c.platform_status,c.source_kind,c.provenance,c.view_url,c.apply_url,c.submit_url,COALESCE(c.verified_at,''),COALESCE(t.user_status,''),COALESCE(t.notes,''),COALESCE(t.is_tracked,0)=1,COALESCE(t.tracked_at,'') FROM campaign_sources c LEFT JOIN tracked_campaigns t ON t.campaign_id=c.id`

// scanner is satisfied by both *sql.Row and *sql.Rows.
type scanner interface{ Scan(dest ...any) error }

func scanCampaign(row scanner) (Campaign, error) {
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
		c, err := scanCampaign(rows)
		if err != nil {
			return nil, err
		}
		campaigns = append(campaigns, c)
	}
	return campaigns, rows.Err()
}

// TrackCampaign starts or resumes local tracking. An empty status keeps the
// existing status (or starts at Interested); empty notes keep existing notes.
func (s *Store) TrackCampaign(ctx context.Context, id, status, notes string) error {
	status = strings.TrimSpace(status)
	if status != "" && !ValidStatus(status) {
		return InputErrorf("invalid tracked campaign status")
	}
	if err := validateNotes(notes); err != nil {
		return err
	}
	ts := now()
	if status == "" {
		var existing string
		err := s.db.QueryRowContext(ctx, `SELECT user_status FROM tracked_campaigns WHERE campaign_id=?`, id).Scan(&existing)
		if errors.Is(err, sql.ErrNoRows) {
			status = Statuses[0]
		} else if err != nil {
			return err
		} else {
			status = existing
		}
	}
	result, err := s.db.ExecContext(ctx, `INSERT INTO tracked_campaigns(campaign_id,user_status,notes,is_tracked,tracked_at,updated_at)
		SELECT id,?,?,1,?,? FROM campaign_sources WHERE id=?
		ON CONFLICT(campaign_id) DO UPDATE SET user_status=excluded.user_status,notes=CASE WHEN excluded.notes='' THEN tracked_campaigns.notes ELSE excluded.notes END,is_tracked=1,updated_at=excluded.updated_at`,
		status, strings.TrimSpace(notes), ts, ts, id)
	return requireRow(result, err)
}

func (s *Store) UntrackCampaign(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `UPDATE tracked_campaigns SET is_tracked=0,updated_at=? WHERE campaign_id=?`, now(), id)
	return requireRow(result, err)
}

// requireRow converts "no rows changed" into ErrCampaignNotFound.
func requireRow(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err == nil && n == 0 {
		return ErrCampaignNotFound
	}
	return err
}

func (s *Store) UpdateManualCampaign(ctx context.Context, id string, input CampaignInput, status, notes string, tracked bool) error {
	in, err := validateCampaignInput(input)
	if err != nil {
		return err
	}
	if tracked && !ValidStatus(status) {
		return InputErrorf("invalid tracked campaign status")
	}
	if tracked {
		if err = validateNotes(notes); err != nil {
			return err
		}
	}
	ts := now()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE campaign_sources SET platform=?,canonical_url=?,title=?,requirements=?,payout_cents=?,budget_cents=?,deadline=?,view_url=?,apply_url=?,submit_url=?,updated_at=? WHERE id=? AND source_kind='manual'`,
		in.Platform, in.URL, in.Title, in.Requirements, in.PayoutCents, in.BudgetCents, nullableString(in.Deadline), in.ViewURL, in.ApplyURL, in.SubmitURL, ts, id)
	if isUniqueViolation(err) {
		return ErrCampaignDuplicate
	}
	if err = requireRow(result, err); err != nil {
		return err
	}
	if tracked {
		if _, err = tx.ExecContext(ctx, `INSERT INTO tracked_campaigns(campaign_id,user_status,notes,is_tracked,tracked_at,updated_at) VALUES(?,?,?,1,?,?) ON CONFLICT(campaign_id) DO UPDATE SET user_status=excluded.user_status,notes=excluded.notes,is_tracked=1,updated_at=excluded.updated_at`, id, status, strings.TrimSpace(notes), ts, ts); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// UpsertDiscoveredCampaign updates only platform-owned fields. The tracking
// row intentionally remains untouched so discovery refreshes preserve notes/status.
func (s *Store) UpsertDiscoveredCampaign(ctx context.Context, c Campaign) (string, error) {
	in, err := validateCampaignInput(CampaignInput{Platform: c.Platform, URL: c.URL, Title: c.Title, Requirements: c.Requirements, PayoutCents: c.PayoutCents, BudgetCents: c.BudgetCents, Deadline: c.Deadline, ViewURL: c.ViewURL, ApplyURL: c.ApplyURL, SubmitURL: c.SubmitURL})
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(c.ExternalID) == "" || len(c.ExternalID) > 256 {
		return "", InputErrorf("discovered campaign needs a stable external ID")
	}
	id, err := newID()
	if err != nil {
		return "", err
	}
	ts := now()
	status, provenance := strings.TrimSpace(c.PlatformStatus), strings.TrimSpace(c.Provenance)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	var existingID string
	err = tx.QueryRowContext(ctx, `SELECT id FROM campaign_sources WHERE platform=? AND external_id=?`, in.Platform, c.ExternalID).Scan(&existingID)
	if err == nil {
		_, err = tx.ExecContext(ctx, `UPDATE campaign_sources SET canonical_url=?,title=?,requirements=?,payout_cents=?,budget_cents=?,deadline=?,platform_status=?,provenance=?,view_url=?,apply_url=?,submit_url=?,verified_at=?,updated_at=? WHERE id=?`,
			in.URL, in.Title, in.Requirements, in.PayoutCents, in.BudgetCents, nullableString(in.Deadline), status, provenance, in.ViewURL, in.ApplyURL, in.SubmitURL, ts, ts, existingID)
		if err != nil {
			return "", err
		}
		return existingID, tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO campaign_sources(id,platform,external_id,canonical_url,title,requirements,payout_cents,budget_cents,deadline,platform_status,source_kind,provenance,view_url,apply_url,submit_url,verified_at,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,'discovered',?,?,?,?,?,?,?)
	ON CONFLICT(platform,canonical_url) DO UPDATE SET external_id=excluded.external_id,title=excluded.title,requirements=excluded.requirements,payout_cents=excluded.payout_cents,budget_cents=excluded.budget_cents,deadline=excluded.deadline,platform_status=excluded.platform_status,provenance=excluded.provenance,view_url=excluded.view_url,apply_url=excluded.apply_url,submit_url=excluded.submit_url,verified_at=excluded.verified_at,updated_at=excluded.updated_at`,
		id, in.Platform, c.ExternalID, in.URL, in.Title, in.Requirements, in.PayoutCents, in.BudgetCents, nullableString(in.Deadline), status, provenance, in.ViewURL, in.ApplyURL, in.SubmitURL, ts, ts, ts)
	if err != nil {
		return "", err
	}
	if err = tx.QueryRowContext(ctx, `SELECT id FROM campaign_sources WHERE platform=? AND canonical_url=?`, in.Platform, in.URL).Scan(&existingID); err != nil {
		return "", err
	}
	return existingID, tx.Commit()
}

func ParseMoneyCents(value string) (*int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	if strings.HasPrefix(value, "-") || strings.HasPrefix(value, "+") {
		return nil, InputErrorf("amount must be non-negative")
	}
	parts := strings.Split(value, ".")
	if len(parts) > 2 || parts[0] == "" || len(parts) == 2 && len(parts[1]) > 2 {
		return nil, InputErrorf("amount must have at most two decimal places")
	}
	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || whole > 90_000_000_000_000 {
		return nil, InputErrorf("amount is out of range")
	}
	fraction := "00"
	if len(parts) == 2 {
		fraction = parts[1] + strings.Repeat("0", 2-len(parts[1]))
	}
	cents, err := strconv.ParseUint(fraction, 10, 64)
	if err != nil {
		return nil, InputErrorf("invalid amount")
	}
	valueCents := whole*100 + int64(cents)
	return &valueCents, nil
}

func FormatMoney(cents *int64) string {
	if cents == nil {
		return "Unknown"
	}
	return fmt.Sprintf("$%d.%02d", *cents/100, *cents%100)
}

// FormatMoneyInput formats cents for an editable form field.
func FormatMoneyInput(cents *int64) string {
	if cents == nil {
		return ""
	}
	return fmt.Sprintf("%d.%02d", *cents/100, *cents%100)
}
