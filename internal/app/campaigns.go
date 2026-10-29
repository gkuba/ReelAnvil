package app

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/gkuba/ReelAnvil/internal/store"
)

type campaignPageData struct {
	Section, View, Search, Platform, Status, Sort, Message, Error string
	BodyTheme                                                     string
	ThemeCSS                                                      template.CSS
	IconURL                                                       string
	SidebarHTML                                                   template.HTML
	SidebarCSS                                                    template.CSS
	SidebarScript                                                 template.JS
	Campaigns                                                     []store.Campaign
	Available, Tracked, EndingSoon                                int
	Platforms                                                     []platformOption
	Statuses                                                      []string
}
type platformOption struct{ ID, Name string }

// CampaignDiscoveryAdapter is the only integration boundary for public
// discovery providers. Build 0.9 wires no adapters because neither platform's
// public discovery API and permitted use have been verified.
type CampaignDiscoveryAdapter interface {
	PlatformID() string
	Discover(context.Context) ([]store.Campaign, error)
}

var campaignPlatforms = []platformOption{{"content-rewards", "Content Rewards"}, {"vyro", "Vyro"}}
var campaignStatuses = []string{"Interested", "Applied", "Active", "Completed", "Archived"}

func (s *Server) campaignSnapshot() ([]store.Campaign, error) {
	if s.store == nil {
		return nil, errors.New("campaign storage is unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return s.store.ListCampaigns(ctx)
}

func (s *Server) campaignConfig() (Settings, template.CSS) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg, template.CSS(themeCSS(s.themes))
}

func (s *Server) campaignsPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	all, err := s.campaignSnapshot()
	if err != nil {
		http.Error(w, "campaign storage is unavailable", http.StatusServiceUnavailable)
		return
	}
	view := r.URL.Query().Get("view")
	if view != "tracked" {
		view = "available"
	}
	search := strings.TrimSpace(r.URL.Query().Get("q"))
	platform := r.URL.Query().Get("platform")
	status := r.URL.Query().Get("status")
	sortKey := r.URL.Query().Get("sort")
	if sortKey != "deadline" && sortKey != "payout" {
		sortKey = "title"
	}
	rows := make([]store.Campaign, 0, len(all))
	available, tracked, ending := campaignCounts(all, time.Now())
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
	store.StableSortCampaigns(rows, func(a, b store.Campaign) bool {
		switch sortKey {
		case "deadline":
			ad, ae := campaignDeadline(a.Deadline)
			bd, be := campaignDeadline(b.Deadline)
			if ae != be {
				return ae
			}
			if ae && !ad.Equal(bd) {
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
	})
	cfg, css := s.campaignConfig()
	icon := iconURL(selectedIcon(cfg), 64)
	data := campaignPageData{Section: "campaigns", View: view, Search: search, Platform: platform, Status: status, Sort: sortKey, Message: r.URL.Query().Get("message"), Error: r.URL.Query().Get("error"), BodyTheme: bodyTheme(cfg.Theme), ThemeCSS: css, IconURL: icon, SidebarHTML: renderSidebar("campaigns", icon), SidebarCSS: template.CSS(sidebarCSS), SidebarScript: template.JS(sidebarScript), Campaigns: rows, Available: available, Tracked: tracked, EndingSoon: ending, Platforms: campaignPlatforms, Statuses: campaignStatuses}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := campaignTemplate.Execute(w, data); err != nil {
		s.log.Println("campaign page render:", err)
	}
}

func (s *Server) addCampaign(w http.ResponseWriter, r *http.Request) {
	if !postOnly(w, r) {
		return
	}
	if s.store == nil {
		http.Error(w, "campaign storage is unavailable", http.StatusServiceUnavailable)
		return
	}
	if err := r.ParseForm(); err != nil {
		redirectCampaign(w, r, "", "invalid form submission")
		return
	}
	in, err := campaignInputFromForm(r)
	if err != nil {
		redirectCampaign(w, r, "", err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	c, created, err := s.store.AddManualCampaign(ctx, in)
	if err != nil {
		redirectCampaign(w, r, "", err.Error())
		return
	}
	if created {
		redirectCampaign(w, r, "Manual campaign added to Tracked with Interested status.", "")
	} else if c.Tracked {
		redirectCampaign(w, r, "This campaign is already tracked.", "")
	} else {
		redirectCampaign(w, r, "This campaign already exists. Use Track to add it to your list.", "")
	}
}

func campaignInputFromForm(r *http.Request) (store.CampaignInput, error) {
	payout, err := store.ParseMoneyCents(r.FormValue("payout"))
	if err != nil {
		return store.CampaignInput{}, fmt.Errorf("payout: %w", err)
	}
	budget, err := store.ParseMoneyCents(r.FormValue("budget"))
	if err != nil {
		return store.CampaignInput{}, fmt.Errorf("budget: %w", err)
	}
	return store.CampaignInput{Platform: r.FormValue("platform"), URL: r.FormValue("url"), Title: r.FormValue("title"), Requirements: r.FormValue("requirements"), PayoutCents: payout, BudgetCents: budget, Deadline: r.FormValue("deadline"), Notes: r.FormValue("notes"), ViewURL: r.FormValue("view_url"), ApplyURL: r.FormValue("apply_url"), SubmitURL: r.FormValue("submit_url")}, nil
}

func redirectCampaign(w http.ResponseWriter, r *http.Request, message, errText string) {
	target := "/campaigns?view=tracked"
	if message != "" {
		target += "&message=" + url.QueryEscape(message)
	}
	if errText != "" {
		target += "&error=" + url.QueryEscape(errText)
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

func (s *Server) trackCampaign(w http.ResponseWriter, r *http.Request) {
	if !postOnly(w, r) {
		return
	}
	if s.store == nil {
		http.Error(w, "campaign storage is unavailable", http.StatusServiceUnavailable)
		return
	}
	if err := r.ParseForm(); err != nil {
		redirectCampaign(w, r, "", "invalid form submission")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	err := s.store.TrackCampaign(ctx, r.FormValue("id"), "", r.FormValue("notes"))
	if err != nil {
		redirectCampaign(w, r, "", err.Error())
		return
	}
	redirectCampaign(w, r, "Campaign tracked with Interested status.", "")
}

func (s *Server) untrackCampaign(w http.ResponseWriter, r *http.Request) {
	if !postOnly(w, r) {
		return
	}
	if s.store == nil {
		http.Error(w, "campaign storage is unavailable", http.StatusServiceUnavailable)
		return
	}
	if err := r.ParseForm(); err != nil {
		redirectCampaign(w, r, "", "invalid form submission")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err := s.store.UntrackCampaign(ctx, r.FormValue("id")); err != nil {
		redirectCampaign(w, r, "", err.Error())
		return
	}
	redirectCampaign(w, r, "Campaign untracked. Its source record remains available.", "")
}

func (s *Server) setCampaignStatus(w http.ResponseWriter, r *http.Request) {
	if !postOnly(w, r) {
		return
	}
	if s.store == nil {
		http.Error(w, "campaign storage is unavailable", http.StatusServiceUnavailable)
		return
	}
	if err := r.ParseForm(); err != nil {
		redirectCampaign(w, r, "", "invalid form submission")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err := s.store.TrackCampaign(ctx, r.FormValue("id"), r.FormValue("status"), r.FormValue("notes")); err != nil {
		redirectCampaign(w, r, "", err.Error())
		return
	}
	redirectCampaign(w, r, "Tracked campaign updated.", "")
}

func (s *Server) updateCampaign(w http.ResponseWriter, r *http.Request) {
	if !postOnly(w, r) {
		return
	}
	if s.store == nil {
		http.Error(w, "campaign storage is unavailable", http.StatusServiceUnavailable)
		return
	}
	if err := r.ParseForm(); err != nil {
		redirectCampaign(w, r, "", "invalid form submission")
		return
	}
	in, err := campaignInputFromForm(r)
	if err != nil {
		redirectCampaign(w, r, "", err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if err := s.store.UpdateManualCampaign(ctx, r.FormValue("id"), in, r.FormValue("status"), r.FormValue("notes"), r.FormValue("tracked") == "true"); err != nil {
		redirectCampaign(w, r, "", err.Error())
		return
	}
	redirectCampaign(w, r, "Campaign details saved.", "")
}

func campaignCounts(campaigns []store.Campaign, now time.Time) (available, tracked, ending int) {
	for _, c := range campaigns {
		if c.Tracked {
			tracked++
			if c.UserStatus == "Archived" {
				continue
			}
		} else {
			available++
		}
		if deadlineWithinSevenDays(c.Deadline, now) {
			ending++
		}
	}
	return
}

func deadlineWithinSevenDays(value string, now time.Time) bool {
	if value == "" {
		return false
	}
	if d, err := time.ParseInLocation("2006-01-02", value, now.Location()); err == nil {
		day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		return !d.Before(day) && !d.After(day.AddDate(0, 0, 7))
	}
	d, err := time.Parse(time.RFC3339, value)
	return err == nil && !d.Before(now) && !d.After(now.Add(7*24*time.Hour))
}
func campaignDeadline(value string) (time.Time, bool) {
	if value == "" {
		return time.Time{}, false
	}
	if d, e := time.Parse("2006-01-02", value); e == nil {
		return d, true
	}
	d, e := time.Parse(time.RFC3339, value)
	return d, e == nil
}
func platformName(id string) string {
	for _, p := range campaignPlatforms {
		if p.ID == id {
			return p.Name
		}
	}
	return "Unknown platform"
}
func deadlineLabel(value string) string {
	if _, ok := campaignDeadline(value); !ok {
		return "Unknown"
	}
	return value
}
func campaignTemplateFuncs() template.FuncMap {
	return template.FuncMap{"platformName": platformName, "deadlineLabel": deadlineLabel, "money": store.FormatMoney, "moneyInput": func(v *int64) string {
		if v == nil {
			return ""
		}
		return fmt.Sprintf("%d.%02d", *v/100, *v%100)
	}, "isManual": func(c store.Campaign) bool { return c.SourceKind == "manual" }, "hasValue": func(v string) bool { return strings.TrimSpace(v) != "" }}
}

func (s *Server) overviewPage(w http.ResponseWriter, r *http.Request) {
	all, err := s.campaignSnapshot()
	if err != nil {
		http.Error(w, "campaign storage is unavailable", http.StatusServiceUnavailable)
		return
	}
	available, tracked, ending := campaignCounts(all, time.Now())
	trackedRows := make([]store.Campaign, 0, tracked)
	for _, c := range all {
		if c.Tracked && c.UserStatus != "Archived" {
			trackedRows = append(trackedRows, c)
		}
	}
	sort.SliceStable(trackedRows, func(i, j int) bool {
		return strings.ToLower(trackedRows[i].Title) < strings.ToLower(trackedRows[j].Title)
	})
	if len(trackedRows) > 5 {
		trackedRows = trackedRows[:5]
	}
	cfg, css := s.campaignConfig()
	icon := iconURL(selectedIcon(cfg), 64)
	data := campaignPageData{Section: "overview", BodyTheme: bodyTheme(cfg.Theme), ThemeCSS: css, IconURL: icon, SidebarHTML: renderSidebar("overview", icon), SidebarCSS: template.CSS(sidebarCSS), SidebarScript: template.JS(sidebarScript), Campaigns: trackedRows, Available: available, Tracked: tracked, EndingSoon: ending}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := overviewTemplate.Execute(w, data); err != nil {
		s.log.Println("overview render:", err)
	}
}

var campaignTemplate = template.Must(template.New("campaigns").Funcs(campaignTemplateFuncs()).Parse(campaignPageHTML))
var overviewTemplate = template.Must(template.New("overview").Funcs(campaignTemplateFuncs()).Parse(overviewPageHTML))
