package app

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gkuba/ReelAnvil/internal/store"
)

type campaignPageData struct {
	pageBase
	View, Search, Platform, Status, Sort string
	Campaigns                            []store.Campaign
	Available, Tracked, EndingSoon       int
	Platforms                            []store.Platform
	Statuses                             []string
}

type overviewPageData struct {
	pageBase
	Campaigns                      []store.Campaign
	Available, Tracked, EndingSoon int
}

// CampaignDiscoveryAdapter is the only integration boundary for public
// discovery providers. Build 0.9 wires no adapters because neither platform's
// public discovery API and permitted use have been verified.
type CampaignDiscoveryAdapter interface {
	PlatformID() string
	Discover(context.Context) ([]store.Campaign, error)
}

const storeTimeout = 5 * time.Second

func (s *Server) campaignSnapshot() ([]store.Campaign, error) {
	if s.store == nil {
		return nil, errors.New("campaign storage is unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), storeTimeout)
	defer cancel()
	return s.store.ListCampaigns(ctx)
}

func (s *Server) campaignsPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	all, err := s.campaignSnapshot()
	if err != nil {
		s.log.Println("list campaigns:", err)
		http.Error(w, "campaign storage is unavailable", http.StatusServiceUnavailable)
		return
	}
	q := r.URL.Query()
	view := q.Get("view")
	if view != "tracked" {
		view = "available"
	}
	search := strings.TrimSpace(q.Get("q"))
	platform := q.Get("platform")
	status := q.Get("status")
	sortKey := q.Get("sort")
	if sortKey != "deadline" && sortKey != "payout" {
		sortKey = "title"
	}
	available, tracked, ending := campaignCounts(all, time.Now())
	rows := make([]store.Campaign, 0, len(all))
	for _, c := range all {
		if view == "available" && c.Tracked || view == "tracked" && !c.Tracked {
			continue
		}
		if platform != "" && platform != c.Platform {
			continue
		}
		if status != "" && status != c.UserStatus {
			continue
		}
		if search != "" && !strings.Contains(strings.ToLower(c.Title+" "+c.Requirements+" "+c.Platform), strings.ToLower(search)) {
			continue
		}
		rows = append(rows, c)
	}
	sort.SliceStable(rows, func(i, j int) bool { return campaignLess(rows[i], rows[j], sortKey) })
	s.render(w, "campaigns", campaignPageData{
		pageBase: s.base(w, r, "Campaigns", "campaigns"),
		View:     view, Search: search, Platform: platform, Status: status, Sort: sortKey,
		Campaigns: rows, Available: available, Tracked: tracked, EndingSoon: ending,
		Platforms: store.Platforms, Statuses: store.Statuses,
	})
}

// campaignLess orders by the chosen key, with unknown values last, then by title and ID.
func campaignLess(a, b store.Campaign, key string) bool {
	switch key {
	case "deadline":
		ad, aok := a.DeadlineDate()
		bd, bok := b.DeadlineDate()
		if aok != bok {
			return aok
		}
		if aok && !ad.Equal(bd) {
			return ad.Before(bd)
		}
	case "payout":
		if (a.PayoutCents == nil) != (b.PayoutCents == nil) {
			return a.PayoutCents != nil
		}
		if a.PayoutCents != nil && *a.PayoutCents != *b.PayoutCents {
			return *a.PayoutCents > *b.PayoutCents
		}
	}
	if !strings.EqualFold(a.Title, b.Title) {
		return strings.ToLower(a.Title) < strings.ToLower(b.Title)
	}
	return a.ID < b.ID
}

// campaignMutation wraps the shared POST handling for campaign forms: method
// and storage checks, form parsing, a store timeout, and a redirect back to the
// tracked list carrying either the success message or a user-safe error.
func (s *Server) campaignMutation(do func(ctx context.Context, r *http.Request) (string, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !postOnly(w, r) {
			return
		}
		if s.store == nil {
			http.Error(w, "campaign storage is unavailable", http.StatusServiceUnavailable)
			return
		}
		if err := r.ParseForm(); err != nil {
			s.redirectCampaign(w, r, "", store.InputErrorf("invalid form submission"))
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), storeTimeout)
		defer cancel()
		message, err := do(ctx, r)
		s.redirectCampaign(w, r, message, err)
	}
}

func (s *Server) redirectCampaign(w http.ResponseWriter, r *http.Request, message string, err error) {
	if err != nil {
		setFlash(w, s.campaignErrorMessage(err), true)
	} else if message != "" {
		setFlash(w, message, false)
	}
	http.Redirect(w, r, "/campaigns?view=tracked", http.StatusSeeOther)
}

// campaignErrorMessage shows validation messages as-is and hides internal
// database errors behind a generic message, logging the detail.
func (s *Server) campaignErrorMessage(err error) string {
	var input store.InputError
	switch {
	case errors.As(err, &input):
		return input.Error()
	case errors.Is(err, store.ErrCampaignNotFound):
		return "That campaign no longer exists."
	case errors.Is(err, store.ErrCampaignDuplicate):
		return "Another campaign on this platform already uses that URL."
	case errors.Is(err, context.DeadlineExceeded):
		return "The database is busy. Please try again."
	default:
		s.log.Println("campaign update failed:", err)
		return "The campaign could not be saved. See logs/reelanvil.log for details."
	}
}

func (s *Server) addCampaign(w http.ResponseWriter, r *http.Request) {
	s.campaignMutation(func(ctx context.Context, r *http.Request) (string, error) {
		in, err := campaignInputFromForm(r)
		if err != nil {
			return "", err
		}
		c, created, err := s.store.AddManualCampaign(ctx, in)
		switch {
		case err != nil:
			return "", err
		case created:
			return "Manual campaign added to Tracked with Interested status.", nil
		case c.Tracked:
			return "This campaign is already tracked.", nil
		default:
			return "This campaign already exists. Use Track to add it to your list.", nil
		}
	})(w, r)
}

func campaignInputFromForm(r *http.Request) (store.CampaignInput, error) {
	payout, err := store.ParseMoneyCents(r.FormValue("payout"))
	if err != nil {
		return store.CampaignInput{}, store.InputErrorf("payout: %v", err)
	}
	budget, err := store.ParseMoneyCents(r.FormValue("budget"))
	if err != nil {
		return store.CampaignInput{}, store.InputErrorf("budget: %v", err)
	}
	return store.CampaignInput{Platform: r.FormValue("platform"), URL: strings.TrimSpace(r.FormValue("url")), Title: r.FormValue("title"), Requirements: r.FormValue("requirements"), PayoutCents: payout, BudgetCents: budget, Deadline: r.FormValue("deadline"), Notes: r.FormValue("notes"), ViewURL: r.FormValue("view_url"), ApplyURL: r.FormValue("apply_url"), SubmitURL: r.FormValue("submit_url")}, nil
}

func (s *Server) trackCampaign(w http.ResponseWriter, r *http.Request) {
	s.campaignMutation(func(ctx context.Context, r *http.Request) (string, error) {
		return "Campaign tracked with Interested status.", s.store.TrackCampaign(ctx, r.FormValue("id"), "", r.FormValue("notes"))
	})(w, r)
}

func (s *Server) untrackCampaign(w http.ResponseWriter, r *http.Request) {
	s.campaignMutation(func(ctx context.Context, r *http.Request) (string, error) {
		return "Campaign untracked. Its source record remains available.", s.store.UntrackCampaign(ctx, r.FormValue("id"))
	})(w, r)
}

func (s *Server) setCampaignStatus(w http.ResponseWriter, r *http.Request) {
	s.campaignMutation(func(ctx context.Context, r *http.Request) (string, error) {
		return "Tracked campaign updated.", s.store.TrackCampaign(ctx, r.FormValue("id"), r.FormValue("status"), r.FormValue("notes"))
	})(w, r)
}

func (s *Server) updateCampaign(w http.ResponseWriter, r *http.Request) {
	s.campaignMutation(func(ctx context.Context, r *http.Request) (string, error) {
		in, err := campaignInputFromForm(r)
		if err != nil {
			return "", err
		}
		return "Campaign details saved.", s.store.UpdateManualCampaign(ctx, r.FormValue("id"), in, r.FormValue("status"), r.FormValue("notes"), r.FormValue("tracked") == "true")
	})(w, r)
}

func campaignCounts(campaigns []store.Campaign, now time.Time) (available, tracked, ending int) {
	for _, c := range campaigns {
		if c.Tracked {
			tracked++
			if c.UserStatus == store.StatusArchived {
				continue
			}
		} else {
			available++
		}
		if deadlineWithinSevenDays(c, now) {
			ending++
		}
	}
	return
}

// deadlineWithinSevenDays compares calendar dates in now's location, so a
// deadline of today counts and one eight days out does not.
func deadlineWithinSevenDays(c store.Campaign, now time.Time) bool {
	d, ok := c.DeadlineDate()
	if !ok {
		return false
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	return !d.Before(today) && !d.After(today.AddDate(0, 0, 7))
}

func platformName(id string) string {
	for _, p := range store.Platforms {
		if p.ID == id {
			return p.Name
		}
	}
	return "Unknown platform"
}
func deadlineLabel(value string) string {
	if _, ok := (store.Campaign{Deadline: value}).DeadlineDate(); !ok {
		return "Unknown"
	}
	return value
}

func (s *Server) overviewPage(w http.ResponseWriter, r *http.Request) {
	all, err := s.campaignSnapshot()
	if err != nil {
		s.log.Println("list campaigns:", err)
		http.Error(w, "campaign storage is unavailable", http.StatusServiceUnavailable)
		return
	}
	available, tracked, ending := campaignCounts(all, time.Now())
	trackedRows := make([]store.Campaign, 0, tracked)
	for _, c := range all {
		if c.Tracked && c.UserStatus != store.StatusArchived {
			trackedRows = append(trackedRows, c)
		}
	}
	sort.SliceStable(trackedRows, func(i, j int) bool { return campaignLess(trackedRows[i], trackedRows[j], "title") })
	if len(trackedRows) > 5 {
		trackedRows = trackedRows[:5]
	}
	s.render(w, "overview", overviewPageData{
		pageBase:  s.base(w, r, "Overview", "overview"),
		Campaigns: trackedRows, Available: available, Tracked: tracked, EndingSoon: ending,
	})
}
