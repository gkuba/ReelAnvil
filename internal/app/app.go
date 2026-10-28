package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gkuba/ReelAnvil/internal/store"
)

type Settings struct {
	Version      int               `json:"version"`
	InstanceName string            `json:"instance_name"`
	Theme        string            `json:"theme"`
	ThemeIcons   map[string]string `json:"theme_icons"`
}
type Server struct {
	mu         sync.Mutex
	cfg        Settings
	cfgPath    string
	started    time.Time
	log        *log.Logger
	themes     map[string]Theme
	themeError string
	store      *store.Store
}

func defaults() Settings {
	return Settings{Version: 2, InstanceName: "ReelAnvil", Theme: "aurora", ThemeIcons: map[string]string{"aurora": "blue-violet", "dracula-dark": "blue-violet", "tomorrow-night": "blue-violet", "nocturne": "blue-violet", "foundry": "blue-violet"}}
}
func paths() error {
	for _, p := range []string{"logs", "settings", "data", "temp"} {
		if err := os.MkdirAll(p, 0700); err != nil {
			return err
		}
	}
	return nil
}
func load(path string) (Settings, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return defaults(), nil
	}
	if err != nil {
		return Settings{}, err
	}
	var c Settings
	if err = json.Unmarshal(b, &c); err != nil {
		return c, err
	}
	if c.Version != 1 && c.Version != 2 {
		return c, fmt.Errorf("unsupported settings version %d", c.Version)
	}
	// Migrate Build 0.1 theme identifiers without invalidating existing settings.
	switch c.Theme {
	case "dark":
		c.Theme = "tomorrow-night"
	case "slate":
		c.Theme = "aurora"
	case "tokyo-night":
		c.Theme = "aurora"
	case "pastel":
		c.Theme = "nocturne"
	case "dracula-lavender":
		c.Theme = "dracula-dark"
	}
	if c.Version == 1 {
		c.Version = 2
	}
	if c.ThemeIcons == nil {
		c.ThemeIcons = map[string]string{}
	}
	for theme := range defaults().ThemeIcons {
		if !validIconVariant(c.ThemeIcons[theme]) {
			c.ThemeIcons[theme] = "blue-violet"
		}
	}
	// Theme selection is validated after custom themes load in Serve.
	return c, nil
}
func validTheme(theme string) bool {
	switch theme {
	case "aurora", "dracula-dark", "tomorrow-night", "nocturne", "foundry":
		return true
	}
	return false
}
func save(path string, c Settings) error {
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	tmp := path + ".tmp"
	if err = os.WriteFile(tmp, b, 0600); err != nil {
		return err
	}
	if err = os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}
func loopbackAddress(address string) error {
	addr, err := net.ResolveTCPAddr("tcp", address)
	if err != nil {
		return fmt.Errorf("invalid listen address: %w", err)
	}
	if addr.IP == nil || !addr.IP.IsLoopback() {
		return errors.New("unauthenticated development server may only listen on a loopback address")
	}
	return nil
}
func localRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; img-src 'self'; style-src 'unsafe-inline'; script-src 'unsafe-inline'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'")
		host, _, err := net.SplitHostPort(r.Host)
		if err != nil {
			host = strings.Trim(r.Host, "[]")
		}
		ip := net.ParseIP(host)
		if !strings.EqualFold(host, "localhost") && (ip == nil || !ip.IsLoopback()) {
			http.Error(w, "local requests only", http.StatusMisdirectedRequest)
			return
		}
		if r.Method == http.MethodPost {
			fetchSite := r.Header.Get("Sec-Fetch-Site")
			if fetchSite != "" && fetchSite != "same-origin" {
				http.Error(w, "cross-origin request rejected", http.StatusForbidden)
				return
			}
			origin := r.Header.Get("Origin")
			// Browsers can serialize an opaque initiator as the Origin value
			// "null". Accept that serialization only when
			// browser Fetch Metadata independently says same-origin. Other opaque
			// Origin values remain rejected; an absent Origin still uses Referer
			// validation and the existing same-origin metadata policy.
			if origin == "null" && fetchSite == "same-origin" {
				origin = ""
			}
			if origin == "" {
				origin = r.Header.Get("Referer")
			}
			if origin != "" {
				u, parseErr := url.Parse(origin)
				scheme := "http"
				if r.TLS != nil {
					scheme = "https"
				}
				if parseErr != nil || u.User != nil || !strings.EqualFold(u.Host, r.Host) || !strings.EqualFold(u.Scheme, scheme) {
					http.Error(w, "cross-origin request rejected", http.StatusForbidden)
					return
				}
			}
			r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		}
		next.ServeHTTP(w, r)
	})
}
func Doctor() error {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	fmt.Println("ReelAnvil local diagnostics")
	fmt.Println("Working directory:", wd)
	fmt.Println("Configuration:", filepath.Join(wd, "settings/config.json"))
	return nil
}
func (s *Server) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.page)
	mux.HandleFunc("/campaigns", s.campaignsPage)
	mux.HandleFunc("/campaigns/add", s.addCampaign)
	mux.HandleFunc("/campaigns/track", s.trackCampaign)
	mux.HandleFunc("/campaigns/untrack", s.untrackCampaign)
	mux.HandleFunc("/campaigns/status", s.setCampaignStatus)
	mux.HandleFunc("/campaigns/update", s.updateCampaign)
	mux.HandleFunc("/themes/icon", s.saveThemeIcon)
	mux.Handle("/assets/branding/", brandingHandler())
	mux.HandleFunc("/favicon.ico", s.favicon)
	mux.HandleFunc("/settings", s.settings)
	mux.HandleFunc("/settings/themes", s.page)
	mux.HandleFunc("/settings/connectors", s.page)
	mux.HandleFunc("/settings/storage", s.page)
	mux.HandleFunc("/themes/refresh", s.refreshThemes)
	mux.HandleFunc("/themes/duplicate", s.duplicateTheme)
	mux.HandleFunc("/themes/save", s.saveTheme)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "method not allowed", 405)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"status":"ok"}`)
	})
	return localRequest(mux)
}

func Serve() error {
	if err := paths(); err != nil {
		return err
	}
	db, err := store.Open("data/reelanvil.db")
	if err != nil {
		return fmt.Errorf("database initialization: %w", err)
	}
	defer db.Close()
	probeCtx, probeCancel := context.WithTimeout(context.Background(), 10*time.Second)
	if err = db.CheckReadWrite(probeCtx); err != nil {
		probeCancel()
		return fmt.Errorf("database read/write verification: %w", err)
	}
	probeCancel()
	cfgPath := "settings/config.json"
	cfg, err := load(cfgPath)
	if err != nil {
		return fmt.Errorf("load settings: %w", err)
	}
	if err = save(cfgPath, cfg); err != nil {
		return err
	}
	file, err := os.OpenFile("logs/reelanvil.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	s := &Server{cfg: cfg, cfgPath: cfgPath, started: time.Now(), log: log.New(file, "", log.LstdFlags|log.LUTC), themes: map[string]Theme{}, store: db}
	s.reloadThemes()
	ensureThemeIcons(&s.cfg, s.themes)
	if !validTheme(s.cfg.Theme) && s.themes[s.cfg.Theme].ID == "" {
		s.cfg.Theme = "aurora"
	}
	address := os.Getenv("REELANVIL_ADDR")
	if address == "" {
		address = "127.0.0.1:8080"
	}
	if err := loopbackAddress(address); err != nil {
		return err
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	defer listener.Close()
	server := &http.Server{Handler: s.handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second}
	s.log.Println("server listening", address)
	fmt.Println("ReelAnvil mockup: http://" + address)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(shutdown)
	}()
	err = server.Serve(listener)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
func (s *Server) settings(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		s.page(w, r)
		return
	}
	if r.Method != "POST" {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", 400)
		return
	}
	name := strings.TrimSpace(r.FormValue("instance_name"))
	theme := r.FormValue("theme")
	if theme == "" {
		s.mu.Lock()
		theme = s.cfg.Theme
		s.mu.Unlock()
	}
	if len(name) == 0 || len(name) > 64 || strings.ContainsAny(name, "\r\n") {
		http.Error(w, "instance name must be 1-64 characters", 400)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if !validTheme(theme) {
		if _, ok := s.themes[theme]; !ok {
			http.Error(w, "invalid theme", 400)
			return
		}
	}
	if s.themeError != "" && theme != s.cfg.Theme {
		http.Error(w, "theme selection locked until themes.json is repaired and refreshed", http.StatusConflict)
		return
	}
	next := s.cfg
	next.Version, next.InstanceName, next.Theme = 2, name, theme
	next.ThemeIcons = cloneThemeIcons(s.cfg.ThemeIcons)
	ensureThemeIcons(&next, s.themes)
	if err := save(s.cfgPath, next); err != nil {
		s.log.Println("settings save failed:", err)
		http.Error(w, "could not save settings", 500)
		return
	}
	s.cfg = next
	target := "/settings?saved=1"
	if r.Form.Has("theme_page") {
		target = "/settings/themes?saved=1"
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}
func (s *Server) page(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "method not allowed", 405)
		return
	}
	path := r.URL.Path
	section := strings.TrimPrefix(path, "/")
	if section == "settings/themes" {
		section = "themes"
	}
	if section == "settings/connectors" {
		section = "connectors"
	}
	if section == "settings/storage" {
		section = "storage"
	}
	if section == "" {
		section = "overview"
	}
	valid := map[string]bool{"overview": true, "jobs": true, "media": true, "review": true, "campaigns": true, "services": true, "diagnostics": true, "settings": true, "themes": true, "connectors": true, "storage": true}
	if !valid[section] {
		http.NotFound(w, r)
		return
	}
	if section == "overview" {
		s.overviewPage(w, r)
		return
	}
	s.mu.Lock()
	cfg := s.cfg
	themes := sortedThemes(s.themes)
	allThemes := append(builtinOptions(), themes...)
	themeError := s.themeError
	editID := r.URL.Query().Get("edit")
	if editID == "" && len(themes) > 0 {
		editID = themes[0].ID
	}
	editTheme := s.themes[editID]
	css := themeCSS(s.themes)
	ensureThemeIcons(&s.cfg, s.themes)
	iconTheme := r.URL.Query().Get("icon_theme")
	if !validTheme(iconTheme) {
		if _, ok := s.themes[iconTheme]; !ok {
			iconTheme = cfg.Theme
		}
	}
	iconVariant := cfg.ThemeIcons[iconTheme]
	if !validIconVariant(iconVariant) {
		iconVariant = "blue-violet"
	}
	iconOptions := iconOptions(iconVariant)
	iconThemeChoices := themeIconChoices(cfg, s.themes, iconTheme)
	s.mu.Unlock()
	editColors := colorFields(editTheme)
	message := r.URL.Query().Get("message")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	data := struct {
		Section          string
		Config           Settings
		Saved            bool
		Uptime           string
		Themes           []Theme
		AllThemes        []Theme
		EditID           string
		EditTheme        Theme
		EditColors       []ColorField
		ThemeCSS         template.CSS
		BodyTheme        string
		ThemeError       string
		ThemeMessage     string
		IconVariant      string
		IconOptions      []IconOption
		IconURL          string
		IconTheme        string
		IconThemeChoices []ThemeIconChoice
	}{section, cfg, r.URL.Query().Get("saved") == "1", time.Since(s.started).Truncate(time.Second).String(), themes, allThemes, editID, editTheme, editColors, css, bodyTheme(cfg.Theme), themeError, message, iconVariant, iconOptions, iconURL(selectedIcon(cfg), 64), iconTheme, iconThemeChoices}
	if err := pageTemplate.Execute(w, data); err != nil {
		s.log.Println("render:", err)
	}
}

var pageTemplate = template.Must(template.New("page").Parse(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>ReelAnvil — {{.Section}}</title><link rel="icon" type="image/png" sizes="64x64" href="{{.IconURL}}"><style>

/* Build 0.3: multi-hue semantic tokens; each theme owns its entire visual language. */
:root{--bg:#0b1020;--sidebar:#10182b;--surface:#151c27;--surface2:#151c27;--surface3:#1b1729;--line:#344467;--text:#cdd8f5;--muted:#9caed0;--accent:#7aa2f7;--accent-text:#0b1733;--secondary:#7dcfff;--tertiary:#bb9af7;--warm:#e0af68;--glow:#263a70;--nav-active:#26375c;--input:#111a2e;--note-bg:#1e2d4b;--note-text:#c6d7ff;--success:#9ece6a;--focus:#bb9af7;--tile-blue:#1b2b49;--tile-teal:#163039;--tile-purple:#292444;--tile-blue-line:#385a88;--tile-teal-line:#326472;--tile-purple-line:#584b82}
body.dracula-dark{--bg:#17181d;--sidebar:#1c1d23;--surface:#18191e;--surface2:#18191e;--surface3:#1b1d23;--line:#44475a;--text:#f2f0f6;--muted:#aaa7b6;--accent:#a897f5;--accent-text:#17181d;--secondary:#8be9fd;--tertiary:#ff79c6;--warm:#f1fa8c;--glow:#221b2d;--nav-active:#30343d;--input:#191b22;--note-bg:#252934;--note-text:#e4d5f6;--success:#50fa7b;--focus:#8be9fd;--tile-blue:#24262e;--tile-teal:#24262e;--tile-purple:#24262e;--tile-blue-line:#44475a;--tile-teal-line:#44475a;--tile-purple-line:#44475a}
body.tomorrow-night{--bg:#0b0d10;--sidebar:#111419;--surface:#12151a;--surface2:#151a16;--surface3:#1c1817;--line:#3a4551;--text:#e2e5e9;--muted:#a3adb9;--accent:#81a2be;--accent-text:#0a121b;--secondary:#b5bd68;--tertiary:#cc99cc;--warm:#f0c674;--glow:#202b36;--nav-active:#28343e;--input:#12161b;--note-bg:#252e34;--note-text:#d2e0e8;--success:#b5bd68;--focus:#f0c674;--tile-blue:#1c2934;--tile-teal:#242d25;--tile-purple:#2d2631;--tile-blue-line:#3a5266;--tile-teal-line:#4a5940;--tile-purple-line:#56465c}

body.nocturne{--bg:#141019;--sidebar:#1b1726;--surface:#1a1526;--surface2:#161c28;--surface3:#22171f;--line:#554166;--text:#eee5f6;--muted:#b8a6c8;--accent:#ad8bfa;--accent-text:#1c112a;--secondary:#8db6d9;--tertiary:#d99cbd;--warm:#e6bc9a;--glow:#372444;--nav-active:#3d2d4f;--input:#1c1525;--note-bg:#35263f;--note-text:#e3cbed;--success:#a8d6b2;--focus:#d1a6f6;--tile-blue:#252d42;--tile-teal:#23383a;--tile-purple:#39283e;--tile-blue-line:#4b5978;--tile-teal-line:#456463;--tile-purple-line:#745373}
body.foundry{--bg:#0d1014;--sidebar:#17202a;--surface:#111a21;--surface2:#13201d;--surface3:#201a14;--line:#465d6c;--text:#e8edf0;--muted:#a4b2ba;--accent:#f0a44c;--accent-text:#1b1510;--secondary:#81a9c4;--tertiary:#70b8a4;--warm:#eac06c;--glow:#3d2d1c;--nav-active:#403328;--input:#1c1b19;--note-bg:#352b22;--note-text:#f1d5b5;--success:#b5c993;--focus:#86a9ba;--tile-blue:#202f38;--tile-teal:#27352f;--tile-purple:#382c26;--tile-blue-line:#49616b;--tile-teal-line:#4d6355;--tile-purple-line:#71543f}
*{box-sizing:border-box}html{background:var(--bg)}body{margin:0;background:radial-gradient(ellipse at 78% 0%,var(--glow) 0,transparent 43%),var(--bg);color:var(--text);font:15px/1.55 system-ui,sans-serif;min-height:100vh;transition:background-color .18s,color .18s}.shell{display:grid;grid-template-columns:248px 1fr;min-height:100vh}aside{background:var(--sidebar);border-right:1px solid var(--line);padding:30px 18px}.brand{font-size:24px;font-weight:800;letter-spacing:.5px;padding:0 12px 24px}.brand span{color:var(--accent)}nav a{display:block;text-decoration:none;color:var(--muted);padding:11px 15px;border-radius:10px;margin:3px 0;text-transform:capitalize}nav a:hover,nav a.active{background:var(--nav-active);color:var(--text)}main{padding:38px clamp(22px,5vw,72px);max-width:1300px;width:100%}.top{display:flex;justify-content:space-between;gap:20px;align-items:center}.eyebrow{text-transform:uppercase;letter-spacing:2px;color:var(--accent);font-size:11px;font-weight:700}h1{font-size:clamp(29px,4vw,45px);margin:8px 0 8px;letter-spacing:-1.2px}p{color:var(--muted)}.pill{border:1px solid var(--line);background:var(--surface);border-radius:50px;padding:7px 13px;color:var(--muted);font-size:12px}.grid{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:18px;margin:28px 0}.card{background:linear-gradient(145deg,var(--surface2),var(--surface));border:1px solid var(--line);border-radius:16px;padding:24px}.grid>.card:nth-child(3n+1){background:linear-gradient(145deg,var(--tile-blue),var(--surface));border-color:var(--tile-blue-line)}.grid>.card:nth-child(3n+2){background:linear-gradient(145deg,var(--tile-teal),var(--surface2));border-color:var(--tile-teal-line)}.grid>.card:nth-child(3n+3){background:linear-gradient(145deg,var(--tile-purple),var(--surface3));border-color:var(--tile-purple-line)}.grid>.card:nth-child(3n+1) .metric{color:var(--accent)}.grid>.card:nth-child(3n+2) .metric{color:var(--secondary)}.grid>.card:nth-child(3n+3) .metric{color:var(--tertiary)}.card h2{font-size:14px;color:var(--muted);font-weight:500;margin:0 0 10px}.metric{font-size:32px;font-weight:750}.muted{color:var(--muted)}.note{padding:16px 20px;border:1px solid var(--line);background:var(--note-bg);border-radius:12px;color:var(--note-text);margin:24px 0}.wide{grid-column:1/-1}label{display:block;margin:18px 0 7px;color:var(--muted)}input,select{width:100%;max-width:460px;padding:12px 14px;background:var(--input);border:1px solid var(--line);border-radius:8px;color:var(--text);font:inherit}button{margin-top:22px;border:0;border-radius:9px;background:var(--accent);color:var(--accent-text);padding:12px 22px;font:inherit;font-weight:700;cursor:pointer}button:hover{filter:brightness(1.12)}:focus-visible{outline:2px solid var(--focus);outline-offset:3px}.color-field{display:flex;align-items:center;gap:12px}.color-field input{max-width:190px}.color-chip{width:28px;height:28px;border:1px solid var(--line);border-radius:6px;flex:none}button:disabled{opacity:.5;cursor:not-allowed}.success{color:var(--success)}footer{color:var(--muted);font-size:12px;margin-top:60px}.theme-preview{margin-top:22px;max-width:460px;padding:16px;background:var(--surface2);border:1px solid var(--line);border-radius:12px}.theme-preview .sample{display:flex;gap:10px;margin-top:12px}.theme-preview .swatch{flex:1;min-height:34px;border-radius:7px;border:1px solid var(--line)}.theme-preview .sample .swatch:nth-child(1){background:var(--bg)}.theme-preview .sample .swatch:nth-child(2){background:var(--sidebar)}.theme-preview .sample .swatch:nth-child(3){background:var(--surface)}.theme-preview .sample .swatch:nth-child(4){background:var(--accent)}.theme-preview .sample .swatch:nth-child(5){background:var(--secondary)}.theme-preview .sample .swatch:nth-child(6){background:var(--tertiary)}.theme-preview .sample .swatch:nth-child(7){background:var(--warm)}.theme-preview small{color:var(--muted)}@media(max-width:760px){.shell{grid-template-columns:1fr}aside{border-right:0;border-bottom:1px solid var(--line);padding:14px}.brand{padding:0 8px 10px}nav{display:flex;flex-wrap:wrap}nav a{padding:8px 10px;font-size:13px}main{padding:22px}.grid{grid-template-columns:1fr}.top{align-items:flex-start;flex-direction:column}}

/* Build 0.5: settings navigation, compact editor, neutral surfaces. */
.brand{display:flex;align-items:center;gap:8px}.brand-mark{width:32px;height:32px;border-radius:7px;flex:none}.icon-options{display:flex;gap:12px;flex-wrap:wrap;margin-top:10px}.icon-choice{display:flex;align-items:center;gap:8px;border:1px solid var(--line);background:var(--surface);padding:8px 12px;border-radius:10px;cursor:pointer}.icon-choice input{width:auto}.icon-choice img{width:42px;height:42px;border-radius:8px}
.settings-nav{display:flex;flex-direction:column;gap:4px;margin-top:12px}.settings-parent{font-weight:700;color:var(--text);padding:9px 12px}.settings-nav a{padding-left:24px}.settings-nav .nav-icon{display:inline-block;width:16px;height:16px;margin-right:8px;vertical-align:-3px;stroke:currentColor;flex:none}.settings-parent{display:flex;align-items:center}.actions{display:flex;gap:14px;align-items:center;margin-top:16px}.actions.between{justify-content:space-between}.actions.end{justify-content:flex-end}.actions button{margin-top:0}.inline-actions{display:flex;align-items:end;gap:14px;flex-wrap:wrap}.inline-actions select{flex:1;min-width:180px}.inline-actions button{margin-top:0}.color-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:10px 24px}.color-item{min-width:0}.color-field input{width:100%;max-width:none}.color-field{gap:8px}.color-chip{width:24px;height:24px}button{padding:8px 14px;margin-top:12px;font-size:14px}.theme-preview{max-width:none;margin-top:16px}body{--surface2:#202a36;--tile-teal:var(--surface);--tile-purple:var(--surface)}.card{background:var(--surface)}@media(max-width:760px){.color-grid{grid-template-columns:1fr}.settings-nav{flex-direction:row;flex-wrap:wrap}.settings-parent{width:100%}}
{{.ThemeCSS}}body{--surface2:var(--surface);--tile-teal:var(--surface);--tile-purple:var(--surface);--tile-blue:var(--surface)}
/* Build 0.6: compact rail, accessible flyout, glass surfaces and restrained actions */
.shell{grid-template-columns:248px minmax(0,1fr);transition:grid-template-columns .16s ease}.shell.rail-compact{grid-template-columns:64px minmax(0,1fr)}#app-sidebar{position:relative;padding:16px 10px;min-width:0;z-index:20}#app-sidebar .rail-head{display:flex;align-items:center;justify-content:space-between;gap:4px;margin-bottom:14px}#app-sidebar .brand{font-size:20px;padding:0 5px;white-space:nowrap}#rail-toggle{margin:0;padding:8px;background:transparent;color:var(--muted);border:1px solid var(--line);display:flex}#app-sidebar .rail-icon{width:18px;height:18px;flex:none;vertical-align:-4px;margin-right:9px}#rail-toggle .rail-icon{margin:0}#app-sidebar nav a,#app-sidebar .nav-group-toggle{display:flex;align-items:center;width:100%;min-height:40px;padding:9px 10px;margin:2px 0;border:0;border-radius:9px;background:transparent;color:var(--muted);text-decoration:none;text-align:left;font:inherit;font-size:13px;cursor:pointer}#app-sidebar nav a:hover,#app-sidebar nav a.active,#app-sidebar .nav-group-toggle:hover,#app-sidebar .nav-group-toggle.group-active{background:var(--nav-active);color:var(--text)}#app-sidebar .nav-group-toggle .rail-icon:last-child{width:14px;height:14px;margin:0 0 0 auto;transition:transform .15s}#app-sidebar .nav-group.open .nav-group-toggle .rail-icon:last-child{transform:rotate(90deg)}#app-sidebar .nav-children{display:none;padding-left:18px}#app-sidebar .nav-group.open .nav-children{display:block}#app-sidebar .nav-children a{font-size:12px}#app-sidebar .rail-flyout{display:none}.rail-compact #app-sidebar .brand,.rail-compact #app-sidebar nav span,.rail-compact #app-sidebar .nav-group-toggle .rail-icon:last-child,.rail-compact #app-sidebar .nav-children{display:none!important}.rail-compact #app-sidebar .rail-head{justify-content:center}.rail-compact #app-sidebar nav a,.rail-compact #app-sidebar .nav-group-toggle{justify-content:center;padding:11px}.rail-compact #app-sidebar .rail-icon{margin:0}.rail-compact #app-sidebar .rail-flyout.visible{display:block;position:absolute;left:100%;top:70px;width:210px;padding:12px;background:var(--surface);border:1px solid var(--line);border-radius:12px;box-shadow:0 18px 45px #0009;z-index:40}.rail-flyout .flyout-title{font-weight:700;margin:3px 6px 9px}.rail-flyout a{display:block!important;padding:9px 12px!important;margin:2px 0!important}.rail-flyout a span{display:inline!important}.rail-flyout a .rail-icon{margin-right:9px!important}/* Shared navigation: explicit flyout links and accent-derived selection across themes. */
#app-sidebar .rail-flyout a,#app-sidebar .rail-flyout a:link,#app-sidebar .rail-flyout a:visited{color:var(--text)!important;text-decoration:none!important;background:transparent;font-weight:500}
#app-sidebar .rail-flyout a:hover,#app-sidebar .rail-flyout a:focus-visible{background:color-mix(in srgb,var(--accent) 15%,var(--sidebar))!important;color:var(--text)!important;text-decoration:none!important}
#app-sidebar .rail-flyout a.active{background:color-mix(in srgb,var(--accent) 25%,var(--sidebar))!important;border-left:3px solid var(--accent);padding-left:9px!important;color:var(--text)!important}
#app-sidebar nav a.active,#app-sidebar .nav-group-toggle.group-active{background:color-mix(in srgb,var(--accent) 24%,var(--sidebar));border-left:3px solid var(--accent);padding-left:7px;color:var(--text)}
#app-sidebar nav a.active .rail-icon,#app-sidebar .nav-group-toggle.group-active .rail-icon{color:var(--accent)}
.card{background:linear-gradient(150deg,color-mix(in srgb,var(--surface) 76%,#000),color-mix(in srgb,var(--surface) 84%,#000))!important;border:1px solid var(--line);box-shadow:0 12px 32px rgba(0,0,0,.22),inset 0 1px rgba(255,255,255,.045)}.grid>.card{border-color:var(--line)!important}.theme-preview{background:rgba(12,16,24,.78);box-shadow:inset 0 1px rgba(255,255,255,.04)}button.secondary-button{background:var(--input);border:1px solid var(--line);color:var(--text);font-weight:500}button .rail-icon,.card h2 .rail-icon,h1 .rail-icon{margin-right:7px;vertical-align:-3px;width:17px;height:17px}button{display:inline-flex;align-items:center;justify-content:center;gap:5px}.actions.end{justify-content:flex-end}body.dracula-dark{--bg:#10131f;--sidebar:#161b2b;--surface:#131722;--input:#101522;--tertiary:#ff79c6}@media(max-width:760px){.shell,.shell.rail-compact{grid-template-columns:1fr}#app-sidebar{border-bottom:1px solid var(--line)}.rail-compact #app-sidebar nav{display:flex;flex-wrap:wrap}.rail-compact #app-sidebar .rail-flyout.visible{top:100%;left:8px}.rail-compact #app-sidebar .brand{display:none}}
</style></head><body class="{{.BodyTheme}}"><div class="shell"><aside id="app-sidebar"><div class="rail-head"><div class="brand"><img class="brand-mark" src="{{.IconURL}}" alt=""><div>REEL<span>ANVIL</span></div></div><button type="button" id="rail-toggle" aria-label="Collapse sidebar" title="Toggle sidebar"><svg class="rail-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect x="3" y="4" width="18" height="16" rx="2"/><path d="M9 4v16"/></svg></button></div><nav aria-label="Main navigation"><a href="/" data-page="overview" title="Overview" {{if eq .Section "overview"}}class="active"{{end}}><svg class="rail-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect x="3" y="3" width="7" height="7" rx="1"/><rect x="14" y="3" width="7" height="7" rx="1"/><rect x="3" y="14" width="7" height="7" rx="1"/><rect x="14" y="14" width="7" height="7" rx="1"/></svg><span>Overview</span></a><div class="nav-group" data-group="media"><button type="button" class="nav-group-toggle" aria-expanded="false" title="Media"><svg class="rail-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect x="3" y="5" width="18" height="14" rx="2"/><path d="m10 9 5 3-5 3z"/></svg><span>Media</span><svg class="rail-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="m9 18 6-6-6-6"/></svg></button><div class="nav-children"><a href="/media" data-page="media" title="Media Library" {{if eq .Section "media"}}class="active"{{end}}><svg class="rail-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect x="3" y="5" width="18" height="14" rx="2"/><path d="m10 9 5 3-5 3z"/></svg><span>Media Library</span></a><a href="/review" data-page="review" title="Review" {{if eq .Section "review"}}class="active"{{end}}><svg class="rail-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect x="3" y="5" width="18" height="14" rx="2"/><path d="m10 9 5 3-5 3z"/></svg><span>Review</span></a></div></div><div class="nav-group" data-group="workflow"><button type="button" class="nav-group-toggle" aria-expanded="false" title="Automation"><svg class="rail-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><circle cx="6" cy="6" r="2"/><circle cx="18" cy="18" r="2"/><path d="M8 6h6a4 4 0 0 1 4 4v6M6 8v10h10"/></svg><span>Automation</span><svg class="rail-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="m9 18 6-6-6-6"/></svg></button><div class="nav-children"><a href="/jobs" data-page="jobs" title="Jobs" {{if eq .Section "jobs"}}class="active"{{end}}><svg class="rail-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><circle cx="6" cy="6" r="2"/><circle cx="18" cy="18" r="2"/><path d="M8 6h6a4 4 0 0 1 4 4v6M6 8v10h10"/></svg><span>Jobs</span></a><a href="/campaigns" data-page="campaigns" title="Campaigns" {{if eq .Section "campaigns"}}class="active"{{end}}><svg class="rail-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><circle cx="6" cy="6" r="2"/><circle cx="18" cy="18" r="2"/><path d="M8 6h6a4 4 0 0 1 4 4v6M6 8v10h10"/></svg><span>Campaigns</span></a><a href="/services" data-page="services" title="Services" {{if eq .Section "services"}}class="active"{{end}}><svg class="rail-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><circle cx="6" cy="6" r="2"/><circle cx="18" cy="18" r="2"/><path d="M8 6h6a4 4 0 0 1 4 4v6M6 8v10h10"/></svg><span>Services</span></a></div></div><a href="/diagnostics" data-page="diagnostics" title="Diagnostics" {{if eq .Section "diagnostics"}}class="active"{{end}}><svg class="rail-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M3 12h4l3-7 4 14 3-7h4"/></svg><span>Diagnostics</span></a><div class="nav-group" data-group="settings"><button type="button" class="nav-group-toggle" aria-expanded="true" title="Settings"><svg class="rail-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><circle cx="12" cy="12" r="3"/><path d="M12 2v3m0 14v3M2 12h3m14 0h3M5 5l2 2m10 10 2 2M19 5l-2 2M7 17l-2 2"/></svg><span>Settings</span><svg class="rail-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="m9 18 6-6-6-6"/></svg></button><div class="nav-children"><a href="/settings" data-page="settings" title="General" {{if eq .Section "settings"}}class="active"{{end}}><svg class="rail-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M4 6h16M4 12h16M4 18h16"/><circle cx="9" cy="6" r="2"/><circle cx="15" cy="12" r="2"/></svg><span>General</span></a><a href="/settings/themes" data-page="themes" title="Themes" {{if eq .Section "themes"}}class="active"{{end}}><svg class="rail-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M12 3a9 9 0 1 0 0 18h2a2 2 0 0 0 1-3.7 2 2 0 0 1 1-3.7h2a3 3 0 0 0 3-3A9 9 0 0 0 12 3z"/><circle cx="7" cy="11" r="1"/><circle cx="12" cy="7" r="1"/></svg><span>Themes</span></a><a href="/settings/connectors" data-page="connectors" title="Connectors" {{if eq .Section "connectors"}}class="active"{{end}}><svg class="rail-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M8 3v5m8-5v5M5 8h14v3a7 7 0 0 1-14 0V8zm7 10v4"/></svg><span>Connectors</span></a><a href="/settings/storage" data-page="storage" title="Paths &amp; Storage" {{if eq .Section "storage"}}class="active"{{end}}><svg class="rail-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect x="3" y="5" width="18" height="14" rx="2"/><path d="M3 12h18"/><circle cx="17" cy="16" r="1"/></svg><span>Paths &amp; Storage</span></a></div></div></nav><div class="rail-flyout" id="rail-flyout" aria-label="Section navigation"></div></aside><main><div class="top"><div class="eyebrow">LOCAL DEVELOPMENT / BUILD 0.9</div><div class="pill">● Mockup · No processing active</div></div>{{if eq .Section "overview"}}<h1>Forge your next clip.</h1><p>Welcome to {{.Config.InstanceName}}. Your future media workflow, in one place.</p><div class="note">Interface prototype: there are no imported videos, active workers, or AI services. All metrics below are actual empty-state counts.</div><div class="grid"><div class="card"><h2>Active jobs</h2><div class="metric">0</div></div><div class="card"><h2>Media assets</h2><div class="metric">0</div></div><div class="card"><h2>Pending reviews</h2><div class="metric">0</div></div><div class="card wide"><h2>Getting started</h2><p>This first build validates the local web shell, navigation, runtime paths, and persistent settings. Ingestion and processing will arrive in later phases.</p></div></div>{{else if eq .Section "settings"}}<h1><svg class="rail-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M4 6h16M4 12h16M4 18h16"/><circle cx="9" cy="6" r="2"/><circle cx="15" cy="12" r="2"/></svg> General</h1><p>Application-wide settings.</p>{{if .Saved}}<div class="note success">Settings saved successfully.</div>{{end}}<div class="card"><h2>Instance</h2><form method="post" action="/settings"><label for="instance_name">Instance name</label><input id="instance_name" name="instance_name" maxlength="64" required value="{{.Config.InstanceName}}"><div class="actions end"><button type="submit"><svg class="rail-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M4 3h14l3 3v15H3V3zM7 3v7h10V3M7 21v-8h10v8"/></svg>Save Settings</button></div></form></div>{{else if eq .Section "connectors"}}<h1><svg class="rail-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M8 3v5m8-5v5M5 8h14v3a7 7 0 0 1-14 0V8zm7 10v4"/></svg> Connectors</h1><p>External services and integrations will be configured here.</p><div class="card"><h2>Connector configuration</h2><label for="connector-endpoint">Service endpoint</label><input id="connector-endpoint" disabled placeholder="Not yet configurable"><label for="connector-key">API credential</label><input id="connector-key" disabled placeholder="Not yet configurable"><p>Connectors are not implemented. No connection is attempted or saved.</p></div>{{else if eq .Section "storage"}}<h1><svg class="rail-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect x="3" y="5" width="18" height="14" rx="2"/><path d="M3 12h18"/><circle cx="17" cy="16" r="1"/></svg> Paths &amp; Storage</h1><p>Media and processing directories will be configured here.</p><div class="card"><h2>Storage locations</h2><label for="media-path">Media library</label><input id="media-path" disabled placeholder="Not yet configurable"><label for="temp-path">Temporary workspace</label><input id="temp-path" disabled placeholder="Not yet configurable"><label for="output-path">Output directory</label><input id="output-path" disabled placeholder="Not yet configurable"><p>Paths are placeholders and cannot be saved yet.</p></div>{{else if eq .Section "themes"}}<h1><svg class="rail-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M12 3a9 9 0 1 0 0 18h2a2 2 0 0 0 1-3.7 2 2 0 0 1 1-3.7h2a3 3 0 0 0 3-3A9 9 0 0 0 12 3z"/><circle cx="7" cy="11" r="1"/><circle cx="12" cy="7" r="1"/></svg> Themes</h1><p>These settings are saved to <code>./settings/config.json</code> in the server working directory.</p>{{if .Saved}}<div class="note success">Settings saved successfully.</div>{{end}}<div class="card"><form method="post" action="/settings"><input type="hidden" name="theme_page" value="1"><input type="hidden" name="instance_name" value="{{.Config.InstanceName}}"><label for="theme">Interface theme</label><select id="theme" name="theme">{{range .AllThemes}}<option value="{{.ID}}" {{if eq $.Config.Theme .ID}}selected{{end}}>{{.Name}}</option>{{end}}</select><div class="theme-preview" aria-live="polite"><strong>Live theme preview</strong><div class="sample" aria-label="Theme preview swatches in order: Background, Sidebar, Panels / Tiles, Primary Accent, Secondary Accent, Tertiary Accent, Quaternary Accent"><div class="swatch" title="Background"></div><div class="swatch" title="Sidebar"></div><div class="swatch" title="Panels / Tiles"></div><div class="swatch" title="Primary Accent"></div><div class="swatch" title="Secondary Accent"></div><div class="swatch" title="Tertiary Accent"></div><div class="swatch" title="Quaternary Accent"></div></div><small>Swatches: Background · Sidebar · Panels / Tiles · Primary · Secondary · Tertiary · Quaternary. Preview applies immediately; save settings to keep it.</small></div><div class="actions end"><button type="submit" class="secondary-button" form="refresh-themes"><svg class="rail-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M20 7v5h-5M4 17v-5h5"/><path d="M5.8 9A7 7 0 0 1 18 6l2 6M4 12l2 6a7 7 0 0 0 12.2-3"/></svg>Refresh Themes</button><button type="submit"><svg class="rail-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M4 3h14l3 3v15H3V3zM7 3v7h10V3M7 21v-8h10v8"/></svg>Save Settings</button></div></form></div><div class="card" style="margin-top:20px"><h2>Theme icon</h2><p>Choose an icon for each interface theme. The selection applies to the sidebar mark and browser favicon. Custom themes start with the Blue-Violet Film Glow icon.</p><form method="post" action="/themes/icon"><label for="icon-theme">Interface theme</label><select id="icon-theme" name="theme_key">{{range .IconThemeChoices}}<option value="{{.ID}}" {{if .Selected}}selected{{end}}>{{.Name}}</option>{{end}}</select><div class="icon-options">{{range .IconOptions}}<label class="icon-choice"><input type="radio" name="icon_variant" value="{{.ID}}" {{if .Selected}}checked{{end}}><img src="{{.PreviewURL}}" alt=""><span>{{.Name}}</span></label>{{end}}</div><div class="actions end"><button type="submit">Save theme icon</button></div></form></div><div class="card" style="margin-top:20px"><h2><svg class="rail-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M3 21l6-2 11-11-4-4L5 15zM14 6l4 4"/></svg> Theme Manager</h2><p>Built-in themes are read-only. Duplicate a theme to customize it. Manual themes live in <code>./settings/themes.json</code>.</p>{{if .ThemeError}}<div class="note" role="alert"><strong>Theme configuration error:</strong> {{.ThemeError}}<p>Built-in themes remain available. A valid last-known-good backup is used when available. The invalid file is preserved. Theme saving is disabled until you correct it and refresh.</p></div>{{end}}{{if .ThemeMessage}}<div class="note success">{{.ThemeMessage}}</div>{{end}}<form method="post" action="/themes/refresh" id="refresh-themes"></form><form method="post" action="/themes/duplicate"><label for="clone-source">Duplicate a theme</label><div class="inline-actions"><select name="source" id="clone-source">{{range .AllThemes}}<option value="{{.ID}}">{{.Name}}</option>{{end}}</select><button type="submit" {{if .ThemeError}}disabled{{end}}>Duplicate Theme</button></div></form><form method="post" action="/themes/save" id="theme-editor"><label for="custom-id">Custom theme</label><select id="custom-id" name="id">{{range .Themes}}<option value="{{.ID}}" {{if eq $.EditID .ID}}selected{{end}}>{{.Name}}</option>{{end}}</select>{{if .EditID}}<label for="custom-name">Theme name</label><input id="custom-name" name="name" required maxlength="64" value="{{.EditTheme.Name}}"><div class="color-grid">{{range .EditColors}}<div class="color-item"><label for="color-{{.Key}}">{{.Label}}</label><div class="color-field"><span class="color-chip" style="background:{{.Value}}"></span><input id="color-{{.Key}}" name="{{.Key}}" value="{{.Value}}" maxlength="7" pattern="#[0-9a-fA-F]{6}" required></div></div>{{end}}</div><div class="theme-preview" id="custom-preview"><strong>Unsaved preview</strong><div class="sample" id="custom-swatches" aria-hidden="true"></div><small>Preview changes do not affect the application until Save Changes.</small></div><div class="actions end"><button type="button" class="secondary-button" id="theme-cancel"><svg class="rail-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M5 5l14 14M19 5 5 19"/></svg>Cancel</button><button type="submit" id="theme-save" {{if .ThemeError}}disabled{{end}}><svg class="rail-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M4 3h14l3 3v15H3V3zM7 3v7h10V3M7 21v-8h10v8"/></svg>Save Changes</button></div>{{else}}<p>Duplicate a theme to create your first editable copy.</p>{{end}}</form></div>{{else if eq .Section "diagnostics"}}<h1>Diagnostics</h1><p>Local server status; no external integrations are being probed.</p><div class="grid"><div class="card"><h2>HTTP server</h2><div class="metric success">Online</div></div><div class="card"><h2>Uptime</h2><div class="metric">{{.Uptime}}</div></div><div class="card"><h2>AI / worker / media</h2><div class="metric">Not configured</div></div></div>{{else}}<h1 style="text-transform:capitalize">{{.Section}}</h1><p>This page is a navigation and visual-layout placeholder for the first development build.</p><div class="note">Not implemented. No jobs, content, integrations, or background processing are running.</div><div class="card"><h2>Upcoming functionality</h2><p>We'll develop this area in a later phase, using the approved ReelAnvil design standards.</p></div>{{end}}<footer>ReelAnvil · Multi-tone theme UI prototype · Local-only · No production services</footer></main></div><script>(function(){const shell=document.querySelector('.shell'),sidebar=document.getElementById('app-sidebar'),toggle=document.getElementById('rail-toggle'),flyout=document.getElementById('rail-flyout');if(!shell||!sidebar)return;let compact=false;try{compact=localStorage.getItem('reelanvil-rail')==='compact'}catch(e){}function hideFlyout(){flyout.classList.remove('visible');flyout.replaceChildren()}function setCompact(v){compact=v;shell.classList.toggle('rail-compact',v);toggle.setAttribute('aria-label',v?'Expand sidebar':'Collapse sidebar');hideFlyout();try{localStorage.setItem('reelanvil-rail',v?'compact':'expanded')}catch(e){}}setCompact(compact);let groups=sidebar.querySelectorAll('.nav-group');groups.forEach(g=>{const active=!!g.querySelector('a.active');g.classList.toggle('open',active);g.querySelector('button').setAttribute('aria-expanded',String(active));g.querySelector('button').classList.toggle('group-active',active);g.querySelector('button').addEventListener('click',()=>{if(compact){if(flyout.classList.contains('visible')&&flyout.dataset.group===g.dataset.group){hideFlyout();return}flyout.dataset.group=g.dataset.group;flyout.replaceChildren();const title=document.createElement('div');title.className='flyout-title';title.textContent=g.querySelector('button span').textContent;flyout.append(title);g.querySelectorAll('.nav-children a').forEach(a=>flyout.append(a.cloneNode(true)));flyout.classList.add('visible')}else{g.classList.toggle('open');g.querySelector('button').setAttribute('aria-expanded',String(g.classList.contains('open')))}})});toggle.addEventListener('click',()=>setCompact(!compact));sidebar.addEventListener('mouseleave',hideFlyout);sidebar.addEventListener('keydown',e=>{if(e.key==='Escape')hideFlyout()});document.addEventListener('click',e=>{if(!sidebar.contains(e.target))hideFlyout()})})();(function(){const iconThemeSelect=document.getElementById("icon-theme");iconThemeSelect?.addEventListener("change",()=>{const u=new URL(location.href);u.searchParams.set("icon_theme",iconThemeSelect.value);u.searchParams.delete("saved");location.assign(u.toString())});const picker=document.getElementById("theme");if(picker){const initial=document.body.className;picker.addEventListener("change",function(){document.body.className=picker.value.startsWith("theme-")?picker.value:(["aurora","dracula-dark","tomorrow-night","nocturne","foundry"].includes(picker.value)?picker.value:"theme-"+picker.value)});window.addEventListener("pageshow",function(){if(!picker.value)document.body.className=initial})}const editor=document.getElementById("theme-editor");if(!editor)return;const select=document.getElementById("custom-id"), fields=[...editor.querySelectorAll(".color-field input")],name=editor.querySelector("#custom-name"),swatches=document.getElementById("custom-swatches");const original=new Map();function remember(){original.set("name",name?.value||"");fields.forEach(i=>original.set(i.name,i.value))}function preview(){swatches.replaceChildren();fields.forEach(i=>{const v=i.value;if(!/^#[0-9a-fA-F]{6}$/.test(v))return;const chip=document.createElement("span");chip.className="swatch";chip.style.backgroundColor=v;swatches.append(chip);i.parentElement.querySelector(".color-chip").style.backgroundColor=v})}remember();fields.forEach(i=>{i.addEventListener("change",preview);i.addEventListener("keydown",e=>{if(e.key==="Enter"){e.preventDefault();i.blur();preview()}})});document.getElementById("theme-cancel")?.addEventListener("click",()=>{if(name)name.value=original.get("name");fields.forEach(i=>i.value=original.get(i.name));preview()});select?.addEventListener("change",()=>{const u=new URL(location.href);u.searchParams.set("edit",select.value);location.assign(u.toString())});preview()})()</script></body></html>`))
