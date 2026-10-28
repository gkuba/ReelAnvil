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
	return c, nil
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
	switch theme {
	case "aurora", "dark", "slate", "pastel":
	default:
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
:root{color-scheme:dark;--bg:#0b101c;--surface:#121b2b;--surface2:#172237;--line:#2a3a56;--text:#edf0ff;--muted:#9aa9c5;--accent:#a69bff;--blue:#6aa9eb}*{box-sizing:border-box}body{margin:0;background:radial-gradient(ellipse at 78% 0%,#25214d 0,transparent 40%),var(--bg);color:var(--text);font:15px/1.55 system-ui,sans-serif;min-height:100vh}body.dark{--accent:#9ba9c5;--blue:#7c91ac}body.slate{--accent:#90a6c7;--blue:#82a9bb}body.pastel{--accent:#d9b3fb;--blue:#9bd7f1}.shell{display:grid;grid-template-columns:248px 1fr;min-height:100vh}aside{background:#0c1423d9;border-right:1px solid var(--line);padding:30px 18px}.brand{font-size:24px;font-weight:800;letter-spacing:.5px;padding:0 12px 24px}.brand span{color:var(--accent)}nav a{display:block;text-decoration:none;color:var(--muted);padding:11px 15px;border-radius:10px;margin:3px 0;text-transform:capitalize}nav a:hover,nav a.active{background:#252d4b;color:var(--text)}main{padding:38px clamp(22px,5vw,72px);max-width:1300px;width:100%}.top{display:flex;justify-content:space-between;gap:20px;align-items:center}.eyebrow{text-transform:uppercase;letter-spacing:2px;color:var(--accent);font-size:11px;font-weight:700}h1{font-size:clamp(29px,4vw,45px);margin:8px 0 8px;letter-spacing:-1.2px}p{color:var(--muted)}.pill{border:1px solid var(--line);border-radius:50px;padding:7px 13px;color:var(--muted);font-size:12px}.grid{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:18px;margin:28px 0}.card{background:linear-gradient(145deg,#1b2539,#111a2b);border:1px solid var(--line);border-radius:16px;padding:24px}.card h2{font-size:14px;color:var(--muted);font-weight:500;margin:0 0 10px}.metric{font-size:32px;font-weight:750}.muted{color:var(--muted)}.note{padding:16px 20px;border:1px solid #55517b;background:#25234275;border-radius:12px;color:#d8d2ff;margin:24px 0}.wide{grid-column:1/-1}label{display:block;margin:18px 0 7px;color:var(--muted)}input,select{width:100%;max-width:460px;padding:12px 14px;background:#0b1323;border:1px solid #394a68;border-radius:8px;color:var(--text);font:inherit}button{margin-top:22px;border:0;border-radius:9px;background:var(--accent);color:#111526;padding:12px 22px;font:inherit;font-weight:700;cursor:pointer}.success{color:#8ed7b8}footer{color:var(--muted);font-size:12px;margin-top:60px}@media(max-width:760px){.shell{grid-template-columns:1fr}aside{border-right:0;border-bottom:1px solid var(--line);padding:14px}.brand{padding:0 8px 10px}nav{display:flex;flex-wrap:wrap}nav a{padding:8px 10px;font-size:13px}main{padding:22px}.grid{grid-template-columns:1fr}.top{align-items:flex-start;flex-direction:column}}</style></head><body class="{{.Config.Theme}}"><div class="shell"><aside><div class="brand">REEL<span>ANVIL</span></div><nav><a href="/" {{if eq .Section "overview"}}class="active"{{end}}>Overview</a><a href="/jobs" {{if eq .Section "jobs"}}class="active"{{end}}>Jobs</a><a href="/media" {{if eq .Section "media"}}class="active"{{end}}>Media Library</a><a href="/review" {{if eq .Section "review"}}class="active"{{end}}>Review</a><a href="/campaigns" {{if eq .Section "campaigns"}}class="active"{{end}}>Campaigns</a><a href="/services" {{if eq .Section "services"}}class="active"{{end}}>Services</a><a href="/diagnostics" {{if eq .Section "diagnostics"}}class="active"{{end}}>Diagnostics</a><a href="/settings" {{if eq .Section "settings"}}class="active"{{end}}>Settings</a></nav></aside><main><div class="top"><div class="eyebrow">LOCAL DEVELOPMENT / BUILD 0.1</div><div class="pill">● Mockup · No processing active</div></div>{{if eq .Section "overview"}}<h1>Forge your next clip.</h1><p>Welcome to {{.Config.InstanceName}}. Your future media workflow, in one place.</p><div class="note">Interface prototype: there are no imported videos, active workers, or AI services. All metrics below are actual empty-state counts.</div><div class="grid"><div class="card"><h2>Active jobs</h2><div class="metric">0</div></div><div class="card"><h2>Media assets</h2><div class="metric">0</div></div><div class="card"><h2>Pending reviews</h2><div class="metric">0</div></div><div class="card wide"><h2>Getting started</h2><p>This first build validates the local web shell, navigation, runtime paths, and persistent settings. Ingestion and processing will arrive in later phases.</p></div></div>{{else if eq .Section "settings"}}<h1>Settings</h1><p>These settings are saved to <code>./settings/config.json</code> in the server working directory.</p>{{if .Saved}}<div class="note success">Settings saved successfully.</div>{{end}}<div class="card"><form method="post" action="/settings"><label for="instance_name">Instance name</label><input id="instance_name" name="instance_name" maxlength="64" required value="{{.Config.InstanceName}}"><label for="theme">Interface theme</label><select id="theme" name="theme"><option value="aurora" {{if eq .Config.Theme "aurora"}}selected{{end}}>Aurora (default)</option><option value="dark" {{if eq .Config.Theme "dark"}}selected{{end}}>Dark</option><option value="slate" {{if eq .Config.Theme "slate"}}selected{{end}}>Slate</option><option value="pastel" {{if eq .Config.Theme "pastel"}}selected{{end}}>Pastel Aurora (experimental)</option></select><div><button type="submit">Save settings</button></div></form></div>{{else if eq .Section "diagnostics"}}<h1>Diagnostics</h1><p>Local server status; no external integrations are being probed.</p><div class="grid"><div class="card"><h2>HTTP server</h2><div class="metric success">Online</div></div><div class="card"><h2>Uptime</h2><div class="metric">{{.Uptime}}</div></div><div class="card"><h2>AI / worker / media</h2><div class="metric">Not configured</div></div></div>{{else}}<h1 style="text-transform:capitalize">{{.Section}}</h1><p>This page is a navigation and visual-layout placeholder for the first development build.</p><div class="note">Not implemented. No jobs, content, integrations, or background processing are running.</div><div class="card"><h2>Upcoming functionality</h2><p>We'll develop this area in a later phase, using the approved ReelAnvil design standards.</p></div>{{end}}<footer>ReelAnvil · Aurora UI prototype · Local-only · No production services</footer></main></div></body></html>`))
