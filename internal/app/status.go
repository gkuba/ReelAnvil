package app

// The Services page and the Transcription page's live model list. Checks run
// with short timeouts, in parallel, so the page stays quick when something is
// down. The web server has no container access, so the container engines are
// reported as "checked by reelanvil doctor" rather than probed here.

import (
	"fmt"
	"net/http"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"time"
)

const (
	repairCommand  = "sudo reelanvil doctor --repair"
	upgradeCommand = "sudo reelanvil upgrade"
	doctorCommand  = "sudo reelanvil doctor"
)

type fact struct{ Label, Value string }

type serviceCard struct {
	Name, Status string
	Level        string // ok, warn or off: text always comes with the colour
	Facts        []fact
}

// actionItem is something the user has to do. Command is the exact command to
// run; Link points to the settings page that fixes it.
type actionItem struct {
	Text, Command, Link, LinkLabel string
}

type servicesPageData struct {
	pageBase
	Ready   bool
	Summary string
	Cards   []serviceCard
	Actions []actionItem
	Checked string
}

func joinNames(names []string) string {
	if len(names) == 0 {
		return "none"
	}
	extra := ""
	if len(names) > maxListedNames {
		extra = fmt.Sprintf(" and %d more", len(names)-maxListedNames)
		names = names[:maxListedNames]
	}
	return strings.Join(names, ", ") + extra
}

func orNotSet(v string) string {
	if v == "" {
		return "not set"
	}
	return v
}

func (s *Server) servicesPage(w http.ResponseWriter, r *http.Request, sec pageSpec, section string) {
	data := servicesPageData{pageBase: s.base(w, r, sec.title, section)}
	s.mu.Lock()
	cfg := s.cfg
	s.mu.Unlock()
	root := effectiveMediaRoot(cfg, s.dataDir)

	var (
		wg   sync.WaitGroup
		oll  ollamaStatus
		whs  whisperStatus
		stor storageReport
	)
	wg.Add(3)
	go func() { defer wg.Done(); oll = probeOllama(cfg.OllamaAddress) }()
	go func() { defer wg.Done(); whs = probeWhisper(cfg.WhisperAddress) }()
	go func() { defer wg.Done(); stor = checkStorage(root, cfg.StorageLimitGB, true) }()
	wg.Wait()

	add := func(c serviceCard, actions []actionItem) {
		data.Cards = append(data.Cards, c)
		data.Actions = append(data.Actions, actions...)
	}

	add(serviceCard{Name: "Web app", Status: "Running", Level: "ok", Facts: []fact{
		{"Version", Version},
		{"Listening on", s.addr},
		{"Uptime", time.Since(s.started).Truncate(time.Second).String()},
		{"Data directory", s.dataDir},
	}}, nil)
	add(ollamaCard(cfg, oll))
	add(whisperCard(cfg, whs))
	add(mediaCard(cfg))
	add(storageCard(root, stor))

	data.Ready = len(data.Actions) == 0
	if data.Ready {
		data.Summary = "Ready. Every check that runs from the web app passed."
	} else {
		data.Summary = fmt.Sprintf("Needs action: %d item(s) below.", len(data.Actions))
	}
	data.Checked = time.Now().Format("15:04:05")
	s.render(w, sec.page, data)
}

func ollamaCard(cfg Settings, st ollamaStatus) (serviceCard, []actionItem) {
	c := serviceCard{Name: "Ollama", Level: "warn", Facts: []fact{{"Address", cfg.OllamaAddress}}}
	if !st.Reachable {
		c.Status = "Not reachable"
		c.Facts = append(c.Facts, fact{"Problem", st.Err})
		return c, []actionItem{{
			Text:    "Ollama is not reachable at " + cfg.OllamaAddress + ". On an installed host this brings the services back; otherwise start Ollama yourself and check the address.",
			Command: repairCommand, Link: "/settings/ai", LinkLabel: "Settings > AI"}}
	}
	c.Facts = append(c.Facts, fact{"Version", st.Version})
	if st.TagsErr != "" {
		c.Status = "Model list unreadable"
		c.Facts = append(c.Facts, fact{"Problem", st.TagsErr})
		return c, []actionItem{{Text: "Ollama answered, but its model list could not be read.", Link: "/settings/ai", LinkLabel: "Settings > AI"}}
	}
	c.Facts = append(c.Facts, fact{"Installed models", fmt.Sprintf("%d (%s)", len(st.Models), joinNames(st.Models))},
		fact{"Hunter model", orNotSet(cfg.HunterModel)}, fact{"Reviewer model", orNotSet(cfg.ReviewerModel)})
	switch {
	case len(st.Models) == 0:
		c.Status = "No models installed"
		return c, []actionItem{{Text: "Ollama has no models yet. Pull one on the AI page.", Link: "/settings/ai", LinkLabel: "Settings > AI"}}
	case cfg.HunterModel == "" || cfg.ReviewerModel == "":
		c.Status = "Model roles not set"
		return c, []actionItem{{Text: "Choose the hunter and reviewer models.", Link: "/settings/ai", LinkLabel: "Settings > AI"}}
	}
	for _, role := range []struct{ name, model string }{{"hunter", cfg.HunterModel}, {"reviewer", cfg.ReviewerModel}} {
		if !slices.Contains(st.Models, role.model) {
			c.Status = "Model not installed"
			return c, []actionItem{{Text: "The " + role.name + " model " + role.model + " is not installed in Ollama. Pull it or choose another.", Link: "/settings/ai", LinkLabel: "Settings > AI"}}
		}
	}
	c.Status, c.Level = "Ready", "ok"
	return c, nil
}

func whisperCard(cfg Settings, st whisperStatus) (serviceCard, []actionItem) {
	c := serviceCard{Name: "Whisper", Level: "warn", Facts: []fact{{"Address", cfg.WhisperAddress}}}
	if !st.Reachable {
		c.Status = "Not reachable"
		c.Facts = append(c.Facts, fact{"Problem", st.Err})
		return c, []actionItem{{
			Text:    "Whisper is not reachable at " + cfg.WhisperAddress + ". On an installed host this brings the services back; otherwise start the service yourself and check the address.",
			Command: repairCommand, Link: "/settings/transcription", LinkLabel: "Settings > Transcription"}}
	}
	c.Status, c.Level = "Ready", "ok"
	if st.InfoErr != "" {
		c.Status = "Connected"
		c.Facts = append(c.Facts, fact{"Service info", "none received (" + st.InfoErr + ")"})
	} else {
		c.Facts = append(c.Facts, fact{"Models reported", fmt.Sprintf("%d (%s)", len(st.Models), joinNames(st.Models))})
	}
	c.Facts = append(c.Facts, fact{"Saved model and language", cfg.WhisperModel + ", " + cfg.WhisperLanguage + " (applied by " + upgradeCommand + ")"})
	return c, nil
}

func mediaCard(cfg Settings) (serviceCard, []actionItem) {
	c := serviceCard{Name: "Media engine", Facts: []fact{{"Engine", cfg.MediaEngine}}}
	if cfg.MediaEngine != EngineHost {
		c.Status, c.Level = "Not checked here", "off"
		c.Facts = append(c.Facts, fact{"Container engine", "the worker owns the containers, so " + doctorCommand + " checks " + cfg.MediaEngine + " and the media container"})
		return c, nil
	}
	var missing []string
	for _, t := range []struct{ name, path string }{{"FFmpeg", cfg.FFmpegPath}, {"yt-dlp", cfg.YtDlpPath}} {
		if found, err := exec.LookPath(t.path); err != nil {
			missing = append(missing, t.name)
			c.Facts = append(c.Facts, fact{t.name, "not found (" + t.path + ")"})
		} else {
			c.Facts = append(c.Facts, fact{t.name, found})
		}
	}
	if len(missing) > 0 {
		c.Status, c.Level = "Programs missing", "warn"
		return c, []actionItem{{Text: strings.Join(missing, " and ") + " not found on this host. Install the program or correct its path.", Link: "/settings/media", LinkLabel: "Settings > Media"}}
	}
	c.Status, c.Level = "Ready", "ok"
	return c, nil
}

func storageCard(root string, st storageReport) (serviceCard, []actionItem) {
	c := serviceCard{Name: "Storage", Level: "warn", Facts: []fact{{"Media root", root}}}
	fix := func(text string) []actionItem {
		return []actionItem{{Text: text, Link: "/settings/storage", LinkLabel: "Settings > Paths & Storage"}}
	}
	switch {
	case !st.Exists:
		c.Status = "Root missing"
		return c, fix("The media root " + root + " does not exist. Create it, or choose another folder.")
	case !st.IsDir:
		c.Status = "Root is not a folder"
		return c, fix("The media root " + root + " is not a directory.")
	}
	if st.FreeKnown {
		c.Facts = append(c.Facts, fact{"Free", humanBytes(int64(st.Free)) + " of " + humanBytes(int64(st.Total))})
	}
	used := humanBytes(st.Used)
	if st.UsedPartial {
		used = "at least " + used
	}
	limit := "no limit"
	if st.LimitGB > 0 {
		limit = fmt.Sprintf("%d GB limit", st.LimitGB)
	}
	c.Facts = append(c.Facts, fact{"Used", used + " (" + limit + ")"})
	switch {
	case !st.Writable:
		c.Status = "Not writable"
		c.Facts = append(c.Facts, fact{"Problem", clip(st.WriteErr, 160)})
		return c, fix("The media root " + root + " is not writable by ReelAnvil.")
	case st.Over():
		c.Status = "Over limit"
		return c, fix("ReelAnvil uses more than the storage limit. Free space or raise the limit.")
	case len(st.Missing) > 0:
		c.Status = "Folders missing"
		c.Facts = append(c.Facts, fact{"Missing folders", strings.Join(st.Missing, ", ")})
		return c, fix("Save the Paths & Storage settings to create the missing folders: " + strings.Join(st.Missing, ", ") + ".")
	}
	c.Status, c.Level = "Ready", "ok"
	return c, nil
}

// ---- Transcription page ----

type transcriptionPageData struct {
	pageBase
	Config  Settings
	Whisper whisperStatus
}

func (s *Server) transcriptionPage(w http.ResponseWriter, r *http.Request, sec pageSpec, section string) {
	data := transcriptionPageData{pageBase: s.base(w, r, sec.title, section)}
	s.mu.Lock()
	data.Config = s.cfg
	s.mu.Unlock()
	data.Whisper = probeWhisper(data.Config.WhisperAddress)
	s.render(w, sec.page, data)
}
