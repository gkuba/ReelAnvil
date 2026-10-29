package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	"unicode/utf8"

	"github.com/gkuba/ReelAnvil/internal/store"
)

// Version is the build label shown in the UI. Release builds set it with
//
//	-ldflags "-X github.com/gkuba/ReelAnvil/internal/app.Version=..."
var Version = "0.9.2-dev"

const DefaultAddr = "127.0.0.1:8080"

// Options configure the web server.
type Options struct {
	// DataDir holds settings/, data/ and logs/. Relative paths are resolved
	// against the current working directory.
	DataDir string
	// Addr is the loopback host:port to listen on.
	Addr string
}

type Settings struct {
	Version      int    `json:"version"`
	InstanceName string `json:"instance_name"`
	Theme        string `json:"theme"`
	IconVariant  string `json:"icon_variant"`
	// ThemeIcons is retained only to preserve older per-theme preferences when
	// upgrading. Build 0.9.1 uses IconVariant as the global icon choice.
	ThemeIcons map[string]string `json:"theme_icons"`
}
type Server struct {
	mu         sync.Mutex
	cfg        Settings
	cfgPath    string
	dataDir    string
	addr       string
	started    time.Time
	log        *log.Logger
	themes     map[string]Theme
	themeError string
	store      *store.Store
}

const settingsVersion = 3
const configPath = "settings/config.json"

func defaults() Settings {
	return Settings{Version: settingsVersion, InstanceName: "ReelAnvil", Theme: defaultTheme, IconVariant: defaultIcon}
}

// file returns name inside the server's data directory.
func (s *Server) file(name string) string { return filepath.Join(s.dataDir, name) }

func prepareDataDir(dir string) error {
	for _, p := range []string{"logs", "settings", "data"} {
		if err := os.MkdirAll(filepath.Join(dir, p), 0700); err != nil {
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
	if c.Version < 1 || c.Version > settingsVersion {
		return c, fmt.Errorf("unsupported settings version %d", c.Version)
	}
	// Migrate retired theme identifiers without invalidating existing settings.
	previousTheme := c.Theme
	if replacement, ok := legacyThemeIDs[c.Theme]; ok {
		c.Theme = replacement
	}
	if !validIconVariant(c.IconVariant) {
		if c.Version < 3 && validIconVariant(c.ThemeIcons[previousTheme]) {
			c.IconVariant = c.ThemeIcons[previousTheme]
		} else if c.Version < 3 && validIconVariant(c.ThemeIcons[c.Theme]) {
			c.IconVariant = c.ThemeIcons[c.Theme]
		} else {
			c.IconVariant = defaultIcon
		}
	}
	c.Version = settingsVersion
	// Theme selection is validated after custom themes load in Serve.
	return c, nil
}
func save(path string, c Settings) error {
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(path, append(b, '\n'))
}
func validInstanceName(name string) bool {
	return name != "" && utf8.RuneCountInString(name) <= 64 && !strings.ContainsAny(name, "\r\n\x00")
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

// contentSecurityPolicy allows only same-origin scripts, styles and images.
// Pages must not use inline <script>, <style> or style="" attributes.
const contentSecurityPolicy = "default-src 'none'; img-src 'self'; style-src 'self'; script-src 'self'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'"

func localRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", contentSecurityPolicy)
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

// Doctor prints local diagnostics without starting the server or changing files.
func Doctor(opts Options) error {
	dataDir, err := filepath.Abs(opts.DataDir)
	if err != nil {
		return err
	}
	addr := opts.Addr
	if addr == "" {
		addr = DefaultAddr
	}
	line := func(label, value string) { fmt.Printf("%-17s %s\n", label+":", value) }
	fmt.Println("ReelAnvil local diagnostics")
	line("Version", Version)
	line("Data directory", dataDir)
	check := func(label, rel string) {
		path := filepath.Join(dataDir, rel)
		status := "present"
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			status = "missing, created on first serve"
		} else if err != nil {
			status = "error: " + err.Error()
		}
		line(label, path+" ("+status+")")
	}
	check("Settings", configPath)
	check("Themes", themePath)
	check("Database", "data/reelanvil.db")
	check("Log", "logs/reelanvil.log")
	if _, err := load(filepath.Join(dataDir, configPath)); err != nil {
		line("Settings check", "invalid: "+err.Error())
	} else {
		line("Settings check", "ok")
	}
	if _, err := parseThemeFile(filepath.Join(dataDir, themePath)); err != nil && !errors.Is(err, os.ErrNotExist) {
		line("Themes check", "invalid: "+err.Error())
	} else {
		line("Themes check", "ok")
	}
	if err := loopbackAddress(addr); err != nil {
		line("Listen address", addr+" - "+err.Error())
	} else {
		line("Listen address", addr+" (loopback)")
	}
	line("Database schema", fmt.Sprintf("version %d expected by this build", store.SchemaVersion))
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
	mux.Handle("/assets/", staticHandler())
	mux.Handle("/assets/branding/", brandingHandler())
	mux.HandleFunc("/assets/themes.css", s.themesStylesheet)
	mux.HandleFunc("/favicon.ico", s.favicon)
	mux.HandleFunc("/settings", s.settings)
	mux.HandleFunc("/themes/refresh", s.refreshThemes)
	mux.HandleFunc("/themes/duplicate", s.duplicateTheme)
	mux.HandleFunc("/themes/save", s.saveTheme)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"status":"ok"}`)
	})
	return localRequest(mux)
}

func Serve(opts Options) error {
	addr := opts.Addr
	if addr == "" {
		addr = DefaultAddr
	}
	// Refuse a non-loopback address before touching any files.
	if err := loopbackAddress(addr); err != nil {
		return err
	}
	dataDir, err := filepath.Abs(opts.DataDir)
	if err != nil {
		return err
	}
	if err := prepareDataDir(dataDir); err != nil {
		return err
	}
	file, err := os.OpenFile(filepath.Join(dataDir, "logs/reelanvil.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	logger := log.New(file, "", log.LstdFlags|log.LUTC)
	db, err := store.Open(filepath.Join(dataDir, "data/reelanvil.db"))
	if err != nil {
		return fmt.Errorf("database initialization: %w", err)
	}
	defer db.Close()
	probeCtx, probeCancel := context.WithTimeout(context.Background(), 10*time.Second)
	err = db.CheckReadWrite(probeCtx)
	probeCancel()
	if err != nil {
		return fmt.Errorf("database read/write verification: %w", err)
	}
	cfgPath := filepath.Join(dataDir, configPath)
	cfg, err := load(cfgPath)
	if err != nil {
		return fmt.Errorf("load settings: %w", err)
	}
	if err = save(cfgPath, cfg); err != nil {
		return err
	}
	s := &Server{cfg: cfg, cfgPath: cfgPath, dataDir: dataDir, addr: addr, started: time.Now(), log: logger, themes: map[string]Theme{}, store: db}
	s.reloadThemes()
	if !validTheme(s.cfg.Theme) && s.themes[s.cfg.Theme].ID == "" {
		s.cfg.Theme = defaultTheme
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	defer listener.Close()
	server := &http.Server{Handler: s.handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second}
	s.log.Printf("server %s listening on %s, data directory %s", Version, addr, dataDir)
	fmt.Printf("ReelAnvil %s: http://%s\nData directory: %s\n", Version, addr, dataDir)
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
	if r.Method == http.MethodGet {
		s.page(w, r)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	target := "/settings"
	if r.Form.Has("theme_page") {
		target = "/settings/themes"
	}
	name := strings.TrimSpace(r.FormValue("instance_name"))
	if !validInstanceName(name) {
		setFlash(w, "Instance name must be 1-64 characters on one line.", true)
		http.Redirect(w, r, target, http.StatusSeeOther)
		return
	}
	theme := r.FormValue("theme")
	iconVariant := r.FormValue("icon_variant")

	s.mu.Lock()
	defer s.mu.Unlock()
	if theme == "" {
		theme = s.cfg.Theme
	}
	if !validTheme(theme) {
		if _, ok := s.themes[theme]; !ok {
			http.Error(w, "invalid theme", http.StatusBadRequest)
			return
		}
	}
	if s.themeError != "" && theme != s.cfg.Theme {
		http.Error(w, "theme selection locked until themes.json is repaired and refreshed", http.StatusConflict)
		return
	}
	next := s.cfg
	next.Version, next.InstanceName, next.Theme = settingsVersion, name, theme
	if r.Form.Has("icon_variant") {
		if !validIconVariant(iconVariant) {
			http.Error(w, "invalid icon variant", http.StatusBadRequest)
			return
		}
		next.IconVariant = iconVariant
	}
	if err := save(s.cfgPath, next); err != nil {
		s.log.Println("settings save failed:", err)
		http.Error(w, "could not save settings", http.StatusInternalServerError)
		return
	}
	s.cfg = next
	setFlash(w, "Settings saved successfully.", false)
	http.Redirect(w, r, target, http.StatusSeeOther)
}
