package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gkuba/ReelAnvil/internal/store"
)

// ---- Media: sources, incoming files and the new-job form ----

type incomingEntry struct {
	Name string
	Size int64
}

type mediaPageData struct {
	pageBase
	Root      string
	Incoming  []incomingEntry
	Sources   []store.Source
	Campaigns []store.Campaign
	Picked    string
}

func (s *Server) mediaPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	cfg := s.config()
	root := effectiveMediaRoot(cfg, s.dataDir)
	ctx, cancel := context.WithTimeout(r.Context(), storeTimeout)
	defer cancel()
	sources, err := s.store.ListSources(ctx)
	if err != nil {
		s.log.Println("list sources:", err)
	}
	all, err := s.store.ListCampaigns(ctx)
	if err != nil {
		s.log.Println("list campaigns:", err)
	}
	var tracked []store.Campaign
	for _, c := range all {
		if c.Tracked {
			tracked = append(tracked, c)
		}
	}
	s.render(w, "media", mediaPageData{
		pageBase: s.base(w, r, "Media", "media"),
		Root:     root, Incoming: listIncoming(root), Sources: sources, Campaigns: tracked,
		Picked: r.URL.Query().Get("campaign"),
	})
}

// listIncoming lists the regular, non-hidden files in incoming/.
func listIncoming(root string) []incomingEntry {
	entries, _ := os.ReadDir(folderPath(root, FolderIncoming))
	var out []incomingEntry
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") || !e.Type().IsRegular() {
			continue
		}
		if fi, err := e.Info(); err == nil {
			out = append(out, incomingEntry{Name: e.Name(), Size: fi.Size()})
		}
	}
	return out
}

func (s *Server) config() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg
}

func (s *Server) createJob(w http.ResponseWriter, r *http.Request) {
	if !postOnly(w, r) {
		return
	}
	id, err := s.newJob(r)
	if err != nil {
		setFlash(w, s.mediaErrorMessage(err), true)
		http.Redirect(w, r, "/media", http.StatusSeeOther)
		return
	}
	s.wakeJobs()
	setFlash(w, fmt.Sprintf("Job %d queued.", id), false)
	http.Redirect(w, r, fmt.Sprintf("/jobs?id=%d", id), http.StatusSeeOther)
}

func (s *Server) newJob(r *http.Request) (int64, error) {
	if err := r.ParseForm(); err != nil {
		return 0, store.InputErrorf("Invalid form submission.")
	}
	if r.PostForm.Get("rights") != "yes" {
		return 0, store.InputErrorf("Confirm that you have the rights to use this media.")
	}
	in := store.JobInput{Origin: r.PostForm.Get("origin"), CampaignID: r.PostForm.Get("campaign")}
	in.ClipCount, _ = strconv.Atoi(r.PostForm.Get("clips"))
	in.MinS, _ = strconv.Atoi(r.PostForm.Get("min_s"))
	in.MaxS, _ = strconv.Atoi(r.PostForm.Get("max_s"))
	switch in.Origin {
	case "file":
		name := r.PostForm.Get("file")
		root := effectiveMediaRoot(s.config(), s.dataDir)
		if _, err := incomingFile(root, name); err != nil {
			return 0, store.InputErrorf("Choose a file from incoming/.")
		}
		in.Ref, in.Title = name, strings.TrimSuffix(name, filepath.Ext(name))
	case "url":
		raw := strings.TrimSpace(r.PostForm.Get("url"))
		u, err := url.Parse(raw)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || len(raw) > 2048 {
			return 0, store.InputErrorf("The video URL must be an https:// link without a user name or password.")
		}
		in.Ref, in.Title = raw, clip(u.Host+u.Path, 120)
	default:
		return 0, store.InputErrorf("Choose a file or a URL.")
	}
	if t := strings.TrimSpace(r.PostForm.Get("title")); t != "" {
		in.Title = clip(t, 160)
	}
	ctx, cancel := context.WithTimeout(r.Context(), storeTimeout)
	defer cancel()
	if in.CampaignID != "" {
		c, err := s.store.CampaignByID(ctx, in.CampaignID)
		if err != nil || !c.Tracked {
			return 0, store.InputErrorf("Choose a tracked campaign.")
		}
	}
	return s.store.CreateJob(ctx, in)
}

func (s *Server) mediaErrorMessage(err error) string {
	var input store.InputError
	switch {
	case errors.As(err, &input):
		return input.Error()
	case errors.Is(err, store.ErrNotFound):
		return "That item no longer exists."
	case errors.Is(err, context.DeadlineExceeded):
		return "The database is busy. Please try again."
	default:
		s.log.Println("media update failed:", err)
		return "The change could not be saved. See logs/reelanvil.log for details."
	}
}

// ---- Jobs ----

type jobsPageData struct {
	pageBase
	Jobs       []store.Job
	Job        *store.Job
	Log        []store.LogLine
	Candidates []store.Candidate
	Clips      []store.Clip
	Active     bool
}

func (s *Server) jobsPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), storeTimeout)
	defer cancel()
	jobs, err := s.store.ListJobs(ctx, 50)
	if err != nil {
		s.log.Println("list jobs:", err)
		http.Error(w, "job storage is unavailable", http.StatusServiceUnavailable)
		return
	}
	data := jobsPageData{pageBase: s.base(w, r, "Jobs", "jobs"), Jobs: jobs}
	for _, j := range jobs {
		if j.State == store.JobQueued || j.State == store.JobRunning {
			data.Active = true
		}
	}
	if id, err := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64); err == nil {
		j, err := s.store.JobByID(ctx, id)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		data.Job = &j
		data.Active = j.State == store.JobQueued || j.State == store.JobRunning
		data.Log, _ = s.store.JobLog(ctx, id, 200)
		data.Candidates, _ = s.store.ListCandidates(ctx, id)
		data.Clips, _ = s.store.ListClips(ctx, "", id)
	}
	s.render(w, "jobs", data)
}

// jobAction wraps the job forms; each redirects back to the job.
func (s *Server) jobAction(do func(ctx context.Context, id int64) (string, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !postOnly(w, r) {
			return
		}
		id, err := strconv.ParseInt(r.PostFormValue("id"), 10, 64)
		if err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), storeTimeout)
		defer cancel()
		msg, err := do(ctx, id)
		if err != nil {
			setFlash(w, s.mediaErrorMessage(err), true)
		} else {
			setFlash(w, msg, false)
		}
		http.Redirect(w, r, fmt.Sprintf("/jobs?id=%d", id), http.StatusSeeOther)
	}
}

func (s *Server) cancelJob(ctx context.Context, id int64) (string, error) {
	if s.cancelRunning(id) {
		return "Stopping the job…", nil
	}
	j, err := s.store.JobByID(ctx, id)
	if err != nil {
		return "", err
	}
	if j.State != store.JobQueued {
		return "", store.InputErrorf("Only queued or running jobs can be cancelled.")
	}
	return "Job cancelled.", s.store.SetJobState(ctx, id, store.JobCancelled, "Cancelled before it started", "")
}

func (s *Server) retryJob(ctx context.Context, id int64) (string, error) {
	if err := s.store.RetryJob(ctx, id); err != nil {
		return "", err
	}
	s.wakeJobs()
	return "Job queued again; finished steps are kept.", nil
}

// ---- Review ----

type reviewPageData struct {
	pageBase
	View                        string
	Clips                       []store.Clip
	Pending, Approved, Rejected int
	Statuses                    []string
}

func (s *Server) reviewPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	view := r.URL.Query().Get("view")
	if view != store.DecisionApproved && view != store.DecisionRejected {
		view = store.DecisionPending
	}
	ctx, cancel := context.WithTimeout(r.Context(), storeTimeout)
	defer cancel()
	all, err := s.store.ListClips(ctx, "", 0)
	if err != nil {
		s.log.Println("list clips:", err)
		http.Error(w, "clip storage is unavailable", http.StatusServiceUnavailable)
		return
	}
	data := reviewPageData{pageBase: s.base(w, r, "Review", "review"), View: view, Statuses: store.SubmissionStatuses}
	for _, c := range all {
		if !c.QAPass {
			continue
		}
		switch c.Decision {
		case store.DecisionApproved:
			data.Approved++
		case store.DecisionRejected:
			data.Rejected++
		default:
			data.Pending++
		}
		if c.Decision == view {
			data.Clips = append(data.Clips, c)
		}
	}
	s.render(w, "review", data)
}

// reviewAction wraps the review forms; each redirects back to the view.
func (s *Server) reviewAction(do func(ctx context.Context, r *http.Request) (string, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !postOnly(w, r) {
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), storeTimeout)
		defer cancel()
		msg, err := do(ctx, r)
		if err != nil {
			setFlash(w, s.mediaErrorMessage(err), true)
		} else {
			setFlash(w, msg, false)
		}
		view := r.PostForm.Get("view")
		if view != store.DecisionApproved && view != store.DecisionRejected {
			view = store.DecisionPending
		}
		http.Redirect(w, r, "/review?view="+view, http.StatusSeeOther)
	}
}

func formID(r *http.Request, key string) int64 {
	id, _ := strconv.ParseInt(r.PostForm.Get(key), 10, 64)
	return id
}

func (s *Server) decideClip(ctx context.Context, r *http.Request) (string, error) {
	d := r.PostForm.Get("decision")
	if err := s.store.DecideClip(ctx, formID(r, "id"), d, strings.TrimSpace(r.PostForm.Get("note"))); err != nil {
		return "", err
	}
	switch d {
	case store.DecisionApproved:
		return "Clip approved. Download it, post it yourself, then record the post here.", nil
	case store.DecisionRejected:
		return "Clip rejected. The file is kept.", nil
	}
	return "Clip moved back to pending.", nil
}

func (s *Server) addPost(ctx context.Context, r *http.Request) (string, error) {
	_, err := s.store.AddSubmission(ctx, formID(r, "clip"), strings.TrimSpace(r.PostForm.Get("url")),
		r.PostForm.Get("status"), strings.TrimSpace(r.PostForm.Get("notes")))
	return "Post recorded.", err
}

func (s *Server) updatePost(ctx context.Context, r *http.Request) (string, error) {
	views, err := optionalInt(r.PostForm.Get("views"))
	if err != nil {
		return "", store.InputErrorf("Views must be a whole number.")
	}
	var earnings *int64
	if v := strings.TrimSpace(r.PostForm.Get("earnings")); v != "" {
		c, err := store.ParseMoneyCents(v)
		if err != nil {
			return "", err
		}
		earnings = c
	}
	err = s.store.UpdateSubmission(ctx, formID(r, "id"), r.PostForm.Get("status"), views, earnings, strings.TrimSpace(r.PostForm.Get("notes")))
	return "Post updated.", err
}

func optionalInt(v string) (*int64, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil, nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	return &n, err
}

// clipFile serves /media/clip/{id}/video, /video?download=1 and /srt.
func (s *Server) clipFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	idText, kind, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/media/clip/"), "/")
	id, err := strconv.ParseInt(idText, 10, 64)
	if err != nil || (kind != "video" && kind != "srt") {
		http.NotFound(w, r)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), storeTimeout)
	c, err := s.store.ClipByID(ctx, id)
	cancel()
	if err != nil {
		http.NotFound(w, r)
		return
	}
	rel := c.Path
	if kind == "srt" {
		rel = c.SRTPath
	}
	root := effectiveMediaRoot(s.config(), s.dataDir)
	p, ok := underRoot(root, rel)
	if !ok {
		http.NotFound(w, r)
		return
	}
	f, err := os.Open(p)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil || !fi.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}
	// Videos outlast the server's 15 s write timeout.
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(30 * time.Minute))
	name := fmt.Sprintf("reelanvil-job%d-clip%d%s", c.JobID, c.ID, filepath.Ext(p))
	if kind == "srt" {
		w.Header().Set("Content-Type", "application/x-subrip; charset=utf-8")
	}
	if kind == "srt" || r.URL.Query().Get("download") == "1" {
		w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	}
	http.ServeContent(w, r, name, fi.ModTime(), f)
}

// underRoot joins a stored relative path to the media root and refuses
// anything that leaves it or passes through a symbolic link.
func underRoot(root, rel string) (string, bool) {
	if rel == "" || filepath.IsAbs(rel) || !filepath.IsLocal(rel) {
		return "", false
	}
	p := filepath.Join(root, rel)
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", false
	}
	real, err := filepath.EvalSymlinks(p)
	if err != nil || real != filepath.Join(realRoot, rel) {
		return "", false
	}
	return p, true
}

// ---- template helpers ----

// clockMS formats milliseconds as m:ss (or h:mm:ss).
func clockMS(ms int64) string {
	d := time.Duration(ms) * time.Millisecond
	h, m, sec := int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, sec)
	}
	return fmt.Sprintf("%d:%02d", m, sec)
}

func elapsed(d time.Duration) string {
	if d <= 0 {
		return ""
	}
	return d.Round(time.Second).String()
}

// when formats a stored RFC 3339 time as local "2006-01-02 15:04:05".
func when(v string) string {
	t, err := time.Parse(time.RFC3339Nano, v)
	if err != nil {
		return v
	}
	return t.Local().Format("2006-01-02 15:04:05")
}
