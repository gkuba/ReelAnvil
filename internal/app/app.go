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
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

type Settings struct {
	Version      int    `json:"version"`
	InstanceName string `json:"instance_name"`
	Theme        string `json:"theme"`
}
type Server struct {
	mu      sync.Mutex
	cfg     Settings
	cfgPath string
	started time.Time
	log     *log.Logger
}

func defaults() Settings { return Settings{Version: 1, InstanceName: "ReelAnvil", Theme: "aurora"} }
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
	if c.Version != 1 {
		return c, fmt.Errorf("unsupported settings version %d", c.Version)
	}
	// Migrate Build 0.1 theme identifiers without invalidating existing settings.
	switch c.Theme {
	case "dark":
		c.Theme = "tomorrow-night"
	case "slate":
		c.Theme = "tokyo-night"
	case "pastel":
		c.Theme = "nocturne"
	}
	if !validTheme(c.Theme) {
		c.Theme = "aurora"
	}
	return c, nil
}
func validTheme(theme string) bool {
	switch theme {
	case "aurora", "dracula-dark", "tomorrow-night", "tokyo-night", "nocturne", "foundry":
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
func Serve() error {
	if err := paths(); err != nil {
		return err
	}
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
	s := &Server{cfg: cfg, cfgPath: cfgPath, started: time.Now(), log: log.New(file, "", log.LstdFlags|log.LUTC)}
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.page)
	mux.HandleFunc("/settings", s.settings)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			http.Error(w, "method not allowed", 405)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"status":"ok"}`)
	})
	address := os.Getenv("REELANVIL_ADDR")
	if address == "" {
		address = "127.0.0.1:8080"
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	defer listener.Close()
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
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
	if len(name) == 0 || len(name) > 64 || strings.ContainsAny(name, "\r\n") {
		http.Error(w, "instance name must be 1-64 characters", 400)
		return
	}
	if !validTheme(theme) {
		http.Error(w, "invalid theme", 400)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	next := Settings{Version: 1, InstanceName: name, Theme: theme}
	if err := save(s.cfgPath, next); err != nil {
		s.log.Println("settings save failed:", err)
		http.Error(w, "could not save settings", 500)
		return
	}
	s.cfg = next
	http.Redirect(w, r, "/settings?saved=1", http.StatusSeeOther)
}
func (s *Server) page(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "method not allowed", 405)
		return
	}
	path := r.URL.Path
	section := strings.TrimPrefix(path, "/")
	if section == "" {
		section = "overview"
	}
	valid := map[string]bool{"overview": true, "jobs": true, "media": true, "review": true, "campaigns": true, "services": true, "diagnostics": true, "settings": true}
	if !valid[section] {
		http.NotFound(w, r)
		return
	}
	s.mu.Lock()
	cfg := s.cfg
	s.mu.Unlock()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	data := struct {
		Section string
		Config  Settings
		Saved   bool
		Uptime  string
	}{section, cfg, r.URL.Query().Get("saved") == "1", time.Since(s.started).Truncate(time.Second).String()}
	if err := pageTemplate.Execute(w, data); err != nil {
		s.log.Println("render:", err)
	}
}

var pageTemplate = template.Must(template.New("page").Parse(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>ReelAnvil — {{.Section}}</title><style>

/* Build 0.3: multi-hue semantic tokens; each theme owns its entire visual language. */
:root{color-scheme:dark;--bg:#0b111c;--sidebar:#101b2b;--surface:#16243b;--surface2:#142d31;--surface3:#27203e;--line:#35527a;--text:#dde6f6;--muted:#9baac3;--accent:#70a8f8;--accent-text:#091528;--secondary:#5dd5bd;--tertiary:#b4a0f5;--warm:#e8bc83;--glow:#172e45;--nav-active:#243654;--input:#101d2f;--note-bg:#19283e;--note-text:#c3d6fa;--success:#5dd5bd;--focus:#b4a0f5;--tile-blue:#16243b;--tile-teal:#142d31;--tile-purple:#27203e;--tile-blue-line:#35527a;--tile-teal-line:#32665e;--tile-purple-line:#514479}
body.dracula-dark{--bg:#0b0c0e;--sidebar:#111215;--surface:#1b1c21;--surface2:#1b2428;--surface3:#26202f;--line:#44424f;--text:#f2f0f6;--muted:#aaa7b6;--accent:#bd93f9;--accent-text:#140f1b;--secondary:#8be9fd;--tertiary:#ff79c6;--warm:#f1fa8c;--glow:#221b2d;--nav-active:#30283c;--input:#131419;--note-bg:#25212d;--note-text:#e4d5f6;--success:#50fa7b;--focus:#8be9fd;--tile-blue:#1a2630;--tile-teal:#1a2927;--tile-purple:#2a2034;--tile-blue-line:#365365;--tile-teal-line:#365c4f;--tile-purple-line:#614774}
body.tomorrow-night{--bg:#0b0d10;--sidebar:#111419;--surface:#1b2027;--surface2:#202822;--surface3:#2b2523;--line:#3a4551;--text:#e2e5e9;--muted:#a3adb9;--accent:#81a2be;--accent-text:#0a121b;--secondary:#b5bd68;--tertiary:#cc99cc;--warm:#f0c674;--glow:#202b36;--nav-active:#28343e;--input:#12161b;--note-bg:#252e34;--note-text:#d2e0e8;--success:#b5bd68;--focus:#f0c674;--tile-blue:#1c2934;--tile-teal:#242d25;--tile-purple:#2d2631;--tile-blue-line:#3a5266;--tile-teal-line:#4a5940;--tile-purple-line:#56465c}
body.tokyo-night{--bg:#0b1020;--sidebar:#10182b;--surface:#18253c;--surface2:#162c35;--surface3:#29233e;--line:#344467;--text:#cdd8f5;--muted:#9caed0;--accent:#7aa2f7;--accent-text:#0b1733;--secondary:#7dcfff;--tertiary:#bb9af7;--warm:#e0af68;--glow:#263a70;--nav-active:#26375c;--input:#111a2e;--note-bg:#1e2d4b;--note-text:#c6d7ff;--success:#9ece6a;--focus:#bb9af7;--tile-blue:#1b2b49;--tile-teal:#163039;--tile-purple:#292444;--tile-blue-line:#385a88;--tile-teal-line:#326472;--tile-purple-line:#584b82}
body.nocturne{--bg:#141019;--sidebar:#1b1726;--surface:#282039;--surface2:#222b3c;--surface3:#34232f;--line:#554166;--text:#eee5f6;--muted:#b8a6c8;--accent:#ad8bfa;--accent-text:#1c112a;--secondary:#8db6d9;--tertiary:#d99cbd;--warm:#e6bc9a;--glow:#372444;--nav-active:#3d2d4f;--input:#1c1525;--note-bg:#35263f;--note-text:#e3cbed;--success:#a8d6b2;--focus:#d1a6f6;--tile-blue:#252d42;--tile-teal:#23383a;--tile-purple:#39283e;--tile-blue-line:#4b5978;--tile-teal-line:#456463;--tile-purple-line:#745373}
body.foundry{--bg:#111213;--sidebar:#1a1918;--surface:#29241f;--surface2:#202a30;--surface3:#322c22;--line:#584a3c;--text:#f0e8db;--muted:#c0ae9b;--accent:#e9a15b;--accent-text:#201308;--secondary:#86a9ba;--tertiary:#d6bb88;--warm:#eac06c;--glow:#3d2d1c;--nav-active:#403328;--input:#1c1b19;--note-bg:#352b22;--note-text:#f1d5b5;--success:#b5c993;--focus:#86a9ba;--tile-blue:#202f38;--tile-teal:#27352f;--tile-purple:#382c26;--tile-blue-line:#49616b;--tile-teal-line:#4d6355;--tile-purple-line:#71543f}
*{box-sizing:border-box}html{background:var(--bg)}body{margin:0;background:radial-gradient(ellipse at 78% 0%,var(--glow) 0,transparent 43%),var(--bg);color:var(--text);font:15px/1.55 system-ui,sans-serif;min-height:100vh;transition:background-color .18s,color .18s}.shell{display:grid;grid-template-columns:248px 1fr;min-height:100vh}aside{background:var(--sidebar);border-right:1px solid var(--line);padding:30px 18px}.brand{font-size:24px;font-weight:800;letter-spacing:.5px;padding:0 12px 24px}.brand span{color:var(--accent)}nav a{display:block;text-decoration:none;color:var(--muted);padding:11px 15px;border-radius:10px;margin:3px 0;text-transform:capitalize}nav a:hover,nav a.active{background:var(--nav-active);color:var(--text)}main{padding:38px clamp(22px,5vw,72px);max-width:1300px;width:100%}.top{display:flex;justify-content:space-between;gap:20px;align-items:center}.eyebrow{text-transform:uppercase;letter-spacing:2px;color:var(--accent);font-size:11px;font-weight:700}h1{font-size:clamp(29px,4vw,45px);margin:8px 0 8px;letter-spacing:-1.2px}p{color:var(--muted)}.pill{border:1px solid var(--line);background:var(--surface);border-radius:50px;padding:7px 13px;color:var(--muted);font-size:12px}.grid{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:18px;margin:28px 0}.card{background:linear-gradient(145deg,var(--surface2),var(--surface));border:1px solid var(--line);border-radius:16px;padding:24px}.grid>.card:nth-child(3n+1){background:linear-gradient(145deg,var(--tile-blue),var(--surface));border-color:var(--tile-blue-line)}.grid>.card:nth-child(3n+2){background:linear-gradient(145deg,var(--tile-teal),var(--surface2));border-color:var(--tile-teal-line)}.grid>.card:nth-child(3n+3){background:linear-gradient(145deg,var(--tile-purple),var(--surface3));border-color:var(--tile-purple-line)}.grid>.card:nth-child(3n+1) .metric{color:var(--accent)}.grid>.card:nth-child(3n+2) .metric{color:var(--secondary)}.grid>.card:nth-child(3n+3) .metric{color:var(--tertiary)}.card h2{font-size:14px;color:var(--muted);font-weight:500;margin:0 0 10px}.metric{font-size:32px;font-weight:750}.muted{color:var(--muted)}.note{padding:16px 20px;border:1px solid var(--line);background:var(--note-bg);border-radius:12px;color:var(--note-text);margin:24px 0}.wide{grid-column:1/-1}label{display:block;margin:18px 0 7px;color:var(--muted)}input,select{width:100%;max-width:460px;padding:12px 14px;background:var(--input);border:1px solid var(--line);border-radius:8px;color:var(--text);font:inherit}button{margin-top:22px;border:0;border-radius:9px;background:var(--accent);color:var(--accent-text);padding:12px 22px;font:inherit;font-weight:700;cursor:pointer}button:hover{filter:brightness(1.12)}:focus-visible{outline:2px solid var(--focus);outline-offset:3px}.success{color:var(--success)}footer{color:var(--muted);font-size:12px;margin-top:60px}.theme-preview{margin-top:22px;max-width:460px;padding:16px;background:var(--surface2);border:1px solid var(--line);border-radius:12px}.theme-preview .sample{display:flex;gap:10px;margin-top:12px}.theme-preview .swatch{flex:1;min-height:34px;border-radius:7px;border:1px solid var(--line)}.theme-preview .sample .swatch:nth-child(1){background:var(--bg)}.theme-preview .sample .swatch:nth-child(2){background:var(--sidebar)}.theme-preview .sample .swatch:nth-child(3){background:var(--surface)}.theme-preview .sample .swatch:nth-child(4){background:var(--accent)}.theme-preview .sample .swatch:nth-child(5){background:var(--secondary)}.theme-preview .sample .swatch:nth-child(6){background:var(--tertiary)}.theme-preview .sample .swatch:nth-child(7){background:var(--warm)}.theme-preview small{color:var(--muted)}@media(max-width:760px){.shell{grid-template-columns:1fr}aside{border-right:0;border-bottom:1px solid var(--line);padding:14px}.brand{padding:0 8px 10px}nav{display:flex;flex-wrap:wrap}nav a{padding:8px 10px;font-size:13px}main{padding:22px}.grid{grid-template-columns:1fr}.top{align-items:flex-start;flex-direction:column}}
</style></head><body class="{{.Config.Theme}}"><div class="shell"><aside><div class="brand">REEL<span>ANVIL</span></div><nav><a href="/" {{if eq .Section "overview"}}class="active"{{end}}>Overview</a><a href="/jobs" {{if eq .Section "jobs"}}class="active"{{end}}>Jobs</a><a href="/media" {{if eq .Section "media"}}class="active"{{end}}>Media Library</a><a href="/review" {{if eq .Section "review"}}class="active"{{end}}>Review</a><a href="/campaigns" {{if eq .Section "campaigns"}}class="active"{{end}}>Campaigns</a><a href="/services" {{if eq .Section "services"}}class="active"{{end}}>Services</a><a href="/diagnostics" {{if eq .Section "diagnostics"}}class="active"{{end}}>Diagnostics</a><a href="/settings" {{if eq .Section "settings"}}class="active"{{end}}>Settings</a></nav></aside><main><div class="top"><div class="eyebrow">LOCAL DEVELOPMENT / BUILD 0.3</div><div class="pill">● Mockup · No processing active</div></div>{{if eq .Section "overview"}}<h1>Forge your next clip.</h1><p>Welcome to {{.Config.InstanceName}}. Your future media workflow, in one place.</p><div class="note">Interface prototype: there are no imported videos, active workers, or AI services. All metrics below are actual empty-state counts.</div><div class="grid"><div class="card"><h2>Active jobs</h2><div class="metric">0</div></div><div class="card"><h2>Media assets</h2><div class="metric">0</div></div><div class="card"><h2>Pending reviews</h2><div class="metric">0</div></div><div class="card wide"><h2>Getting started</h2><p>This first build validates the local web shell, navigation, runtime paths, and persistent settings. Ingestion and processing will arrive in later phases.</p></div></div>{{else if eq .Section "settings"}}<h1>Settings</h1><p>These settings are saved to <code>./settings/config.json</code> in the server working directory.</p>{{if .Saved}}<div class="note success">Settings saved successfully.</div>{{end}}<div class="card"><form method="post" action="/settings"><label for="instance_name">Instance name</label><input id="instance_name" name="instance_name" maxlength="64" required value="{{.Config.InstanceName}}"><label for="theme">Interface theme</label><select id="theme" name="theme"><option value="aurora" {{if eq .Config.Theme "aurora"}}selected{{end}}>Aurora (default)</option><option value="dracula-dark" {{if eq .Config.Theme "dracula-dark"}}selected{{end}}>Dracula Dark</option><option value="tomorrow-night" {{if eq .Config.Theme "tomorrow-night"}}selected{{end}}>Tomorrow Night</option><option value="tokyo-night" {{if eq .Config.Theme "tokyo-night"}}selected{{end}}>Tokyo Night</option><option value="nocturne" {{if eq .Config.Theme "nocturne"}}selected{{end}}>Nocturne</option><option value="foundry" {{if eq .Config.Theme "foundry"}}selected{{end}}>Foundry</option></select><div class="theme-preview" aria-live="polite"><strong>Live theme preview</strong><div class="sample" aria-hidden="true"><div class="swatch"></div><div class="swatch"></div><div class="swatch"></div><div class="swatch"></div><div class="swatch"></div><div class="swatch"></div><div class="swatch"></div></div><small>Preview applies to this page immediately. Save settings to keep it.</small></div><div><button type="submit">Save settings</button></div></form></div>{{else if eq .Section "diagnostics"}}<h1>Diagnostics</h1><p>Local server status; no external integrations are being probed.</p><div class="grid"><div class="card"><h2>HTTP server</h2><div class="metric success">Online</div></div><div class="card"><h2>Uptime</h2><div class="metric">{{.Uptime}}</div></div><div class="card"><h2>AI / worker / media</h2><div class="metric">Not configured</div></div></div>{{else}}<h1 style="text-transform:capitalize">{{.Section}}</h1><p>This page is a navigation and visual-layout placeholder for the first development build.</p><div class="note">Not implemented. No jobs, content, integrations, or background processing are running.</div><div class="card"><h2>Upcoming functionality</h2><p>We'll develop this area in a later phase, using the approved ReelAnvil design standards.</p></div>{{end}}<footer>ReelAnvil · Multi-tone six-theme UI prototype · Local-only · No production services</footer></main></div><script>(function(){const picker=document.getElementById("theme");if(!picker)return;const initial=document.body.className;picker.addEventListener("change",function(){document.body.className=picker.value});window.addEventListener("pageshow",function(){if(!picker.value)document.body.className=initial})})()</script></body></html>`))
