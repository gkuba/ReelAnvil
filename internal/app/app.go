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
	"sort"
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

// DefaultAddr listens on every interface so the instance is reachable on the
// LAN. It has no authentication: trusted networks only.
const DefaultAddr = "0.0.0.0:4747"

// Options configure the web server.
type Options struct {
	// DataDir holds settings/, data/ and logs/. Relative paths are resolved
	// against the current working directory.
	DataDir string
	// Addr overrides the saved listen address (Settings > General) for this
	// run when not empty.
	Addr string
	// AllowedHosts overrides the saved extra host names when not nil. IP
	// addresses and this machine's host name are always accepted on a
	// non-loopback address.
	AllowedHosts []string
}

type Settings struct {
	Version      int    `json:"version"`
	InstanceName string `json:"instance_name"`
	Theme        string `json:"theme"`
	IconVariant  string `json:"icon_variant"`
	// ThemeIcons is retained only to preserve older per-theme preferences when
	// upgrading. Build 0.9.1 uses IconVariant as the global icon choice.
	ThemeIcons map[string]string `json:"theme_icons"`
	// ListenAddress and AllowedHosts take effect on the next start or restart.
	ListenAddress string   `json:"listen_address"`
	AllowedHosts  []string `json:"allowed_hosts,omitempty"`
	// Container services (Settings > AI, Transcription, Media). Addresses are
	// host:port; the paths are used only with the "host" media engine.
	OllamaAddress  string `json:"ollama_address"`
	WhisperAddress string `json:"whisper_address"`
	MediaEngine    string `json:"media_engine"`
	FFmpegPath     string `json:"ffmpeg_path"`
	YtDlpPath      string `json:"ytdlp_path"`
	// Model roles (Settings > AI): installed Ollama model names, empty = unset.
	HunterModel   string `json:"hunter_model"`
	ReviewerModel string `json:"reviewer_model"`
	// Whisper model and language; the container reads them on `reelanvil upgrade`.
	WhisperModel    string `json:"whisper_model"`
	WhisperLanguage string `json:"whisper_language"`
	// MediaRoot holds incoming/, temp/, completed/, failed/ and archive/.
	// StorageLimitGB caps the space ReelAnvil may use under it; 0 = no limit.
	MediaRoot      string `json:"media_root"`
	StorageLimitGB int    `json:"storage_limit_gb"`
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
	hosts      hostPolicy
	// runHosts are the allowed host names in use; the override flags mark
	// values that came from --addr / --allowed-hosts or the environment.
	runHosts                    []string
	addrOverride, hostsOverride bool
	restart                     chan struct{}
	restartOnce                 sync.Once
	// Model pull running against Ollama, and the context it runs under, which
	// ends when the server shuts down.
	pullMu     sync.Mutex
	pull       pullState
	pullCancel context.CancelFunc
	bgOnce     sync.Once
	bgCtx      context.Context
	bgStop     context.CancelFunc
	bgWG       sync.WaitGroup
	// jobs is the media job being run (guarded by pullMu).
	jobs        jobRunner
	workerToken string
}

const settingsVersion = 5
const configPath = "settings/config.json"

func defaults() Settings {
	return Settings{Version: settingsVersion, InstanceName: "ReelAnvil", Theme: defaultTheme, IconVariant: defaultIcon, ListenAddress: DefaultAddr,
		OllamaAddress: DefaultOllamaAddr, WhisperAddress: DefaultWhisperAddr, MediaEngine: EnginePodman, FFmpegPath: DefaultFFmpegPath, YtDlpPath: DefaultYtDlpPath,
		WhisperModel: DefaultWhisperModel, WhisperLanguage: DefaultWhisperLang}
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
	if c.ListenAddress == "" {
		c.ListenAddress = DefaultAddr
	} else if err := validListenAddress(c.ListenAddress); err != nil {
		return c, fmt.Errorf("invalid listen_address %q: %w", c.ListenAddress, err)
	}
	if c.AllowedHosts, err = parseAllowedHosts(strings.Join(c.AllowedHosts, ",")); err != nil {
		return c, fmt.Errorf("invalid allowed_hosts: %w", err)
	}
	if c.InstanceName == "" {
		c.InstanceName = defaults().InstanceName
	}
	if err := c.fillServices(); err != nil {
		return c, err
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

// exposedAddress reports whether address listens beyond loopback: on all
// interfaces (0.0.0.0, [::] or an empty host) or on a network address.
func exposedAddress(address string) (bool, error) {
	addr, err := net.ResolveTCPAddr("tcp", address)
	if err != nil {
		return false, fmt.Errorf("invalid listen address: %w", err)
	}
	return addr.IP == nil || !addr.IP.IsLoopback(), nil
}

// hostPolicy decides which Host headers are served. The zero value accepts
// only localhost and loopback IPs. An exposed server also accepts IP literals
// and the listed names; other names are still rejected, which keeps the
// DNS-rebinding defense (a rebinding page always uses the attacker's name).
type hostPolicy struct {
	exposed bool
	names   map[string]bool
}

func newHostPolicy(exposed bool, names []string) hostPolicy {
	p := hostPolicy{exposed: exposed, names: map[string]bool{}}
	if !exposed {
		return p
	}
	if h, err := os.Hostname(); err == nil && h != "" {
		h = normalizeHost(h)
		p.names[h] = true
		if !strings.Contains(h, ".") {
			p.names[h+".local"] = true
		}
	}
	for _, n := range names {
		if n = normalizeHost(n); n != "" {
			p.names[n] = true
		}
	}
	return p
}

func normalizeHost(host string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
}

func (p hostPolicy) allows(host string) bool {
	host = normalizeHost(host)
	ip := net.ParseIP(host)
	if host == "localhost" || (ip != nil && ip.IsLoopback()) {
		return true
	}
	return p.exposed && (ip != nil || p.names[host])
}

// exposedWarning is printed and logged whenever the server listens beyond loopback.
const exposedWarning = "WARNING: listening beyond loopback with no authentication. Anyone who can reach this port can use and change this instance. Use it only on a trusted local network; never expose it to the internet."

// contentSecurityPolicy allows only same-origin scripts, styles and images.
// Pages must not use inline <script>, <style> or style="" attributes.
const contentSecurityPolicy = "default-src 'none'; img-src 'self'; media-src 'self'; style-src 'self'; script-src 'self'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'"

func localRequest(next http.Handler, hosts hostPolicy) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Content-Security-Policy", contentSecurityPolicy)
		host, _, err := net.SplitHostPort(r.Host)
		if err != nil {
			host = strings.Trim(r.Host, "[]")
		}
		if !hosts.allows(host) {
			http.Error(w, "host not allowed", http.StatusMisdirectedRequest)
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
	cfg, err := load(filepath.Join(dataDir, configPath))
	if err != nil {
		line("Settings check", "invalid: "+err.Error())
		cfg = defaults()
	} else {
		line("Settings check", "ok")
	}
	addr, hosts := cfg.ListenAddress, cfg.AllowedHosts
	if opts.Addr != "" {
		addr = opts.Addr
	}
	if opts.AllowedHosts != nil {
		hosts = opts.AllowedHosts
	}
	if _, err := parseThemeFile(filepath.Join(dataDir, themePath)); err != nil && !errors.Is(err, os.ErrNotExist) {
		line("Themes check", "invalid: "+err.Error())
	} else {
		line("Themes check", "ok")
	}
	if exposed, err := exposedAddress(addr); err != nil {
		line("Listen address", addr+" - "+err.Error())
	} else if exposed {
		line("Listen address", addr+" (network: no authentication, trusted LAN only)")
		names := make([]string, 0)
		for n := range newHostPolicy(true, hosts).names {
			names = append(names, n)
		}
		sort.Strings(names)
		line("Allowed hosts", "IP addresses, localhost, "+strings.Join(names, ", "))
	} else {
		line("Listen address", addr+" (loopback)")
	}
	root := effectiveMediaRoot(cfg, dataDir)
	if fi, err := os.Stat(root); err != nil || !fi.IsDir() {
		line("Media root", root+" (missing or not a folder)")
	} else {
		line("Media root", root+" (present)")
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
	mux.HandleFunc("/media", s.mediaPage)
	mux.HandleFunc("/media/jobs", s.createJob)
	mux.HandleFunc("/media/clip/", s.clipFile)
	mux.HandleFunc("/jobs", s.jobsPage)
	mux.HandleFunc("/jobs/cancel", s.jobAction(s.cancelJob))
	mux.HandleFunc("/jobs/retry", s.jobAction(s.retryJob))
	mux.HandleFunc("/review", s.reviewPage)
	mux.HandleFunc("/review/decide", s.reviewAction(s.decideClip))
	mux.HandleFunc("/review/post", s.reviewAction(s.addPost))
	mux.HandleFunc("/review/post-update", s.reviewAction(s.updatePost))
	mux.Handle("/assets/", staticHandler())
	mux.Handle("/assets/branding/", brandingHandler())
	mux.HandleFunc("/assets/themes.css", s.themesStylesheet)
	mux.HandleFunc("/favicon.ico", s.favicon)
	mux.HandleFunc("/settings", s.settings)
	mux.HandleFunc("/settings/network", s.saveNetwork)
	mux.HandleFunc("/settings/ai", s.aiSettings)
	mux.HandleFunc("/settings/ai/test", s.aiTest)
	mux.HandleFunc("/settings/ai/model-test", s.aiModelTest)
	mux.HandleFunc("/settings/ai/pull", s.aiPull)
	mux.HandleFunc("/settings/ai/pull/cancel", s.aiPullCancel)
	mux.HandleFunc("/settings/transcription", s.transcriptionSettings)
	mux.HandleFunc("/settings/transcription/test", s.transcriptionTest)
	mux.HandleFunc("/settings/media", s.mediaSettings)
	mux.HandleFunc("/settings/media/test", s.mediaTest)
	mux.HandleFunc("/settings/storage", s.storageSettings)
	mux.HandleFunc("/settings/storage/check", s.storageCheck)
	mux.HandleFunc("/system/restart", s.restartHandler)
	mux.HandleFunc("/themes/refresh", s.refreshThemes)
	mux.HandleFunc("/themes/duplicate", s.duplicateTheme)
	mux.HandleFunc("/themes/save", s.saveTheme)
	mux.HandleFunc("/api/worker/claim", s.workerAuth(s.workerClaim))
	mux.HandleFunc("/api/worker/ops/", s.workerAuth(s.workerOp))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"status":"ok"}`)
	})
	return localRequest(mux, s.hosts)
}

// displayAddr turns a wildcard listener such as 0.0.0.0:4747 into a URL host a
// browser on this machine can open.
func displayAddr(a net.Addr) string {
	tcp, ok := a.(*net.TCPAddr)
	if !ok || !tcp.IP.IsUnspecified() {
		return a.String()
	}
	host := "localhost"
	if h, err := os.Hostname(); err == nil && h != "" {
		host = h
	}
	return net.JoinHostPort(host, fmt.Sprint(tcp.Port))
}

func Serve(opts Options) error {
	// Validate command-line values before touching any files.
	addr, addrOverride := opts.Addr, opts.Addr != ""
	if addrOverride {
		if _, err := exposedAddress(addr); err != nil {
			return err
		}
	}
	hosts, hostsOverride := opts.AllowedHosts, opts.AllowedHosts != nil
	if hostsOverride {
		var err error
		if hosts, err = parseAllowedHosts(strings.Join(hosts, ",")); err != nil {
			return fmt.Errorf("--allowed-hosts: %w", err)
		}
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
	if !addrOverride {
		addr = cfg.ListenAddress
	}
	if !hostsOverride {
		hosts = cfg.AllowedHosts
	}
	exposed, err := exposedAddress(addr)
	if err != nil {
		return err
	}
	s := &Server{cfg: cfg, cfgPath: cfgPath, dataDir: dataDir, addr: addr, started: time.Now(), log: logger, themes: map[string]Theme{}, store: db,
		hosts: newHostPolicy(exposed, hosts), runHosts: hosts, addrOverride: addrOverride, hostsOverride: hostsOverride, restart: make(chan struct{})}
	s.reloadThemes()
	if cfg.MediaRoot == "" {
		// No saved media root: use <data-dir>/media and make sure it is ready.
		root := effectiveMediaRoot(cfg, dataDir)
		err := os.MkdirAll(root, folderMode)
		if err == nil {
			_, err = ensureMediaFolders(root)
		}
		if err != nil {
			s.log.Printf("media root %s: %v", root, err)
		}
	}
	if !validTheme(s.cfg.Theme) && s.themes[s.cfg.Theme].ID == "" {
		s.cfg.Theme = defaultTheme
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	defer listener.Close()
	if s.workerToken, err = ensureWorkerToken(dataDir); err != nil {
		return fmt.Errorf("worker token: %w", err)
	}
	s.startJobs()
	server := &http.Server{Handler: s.handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second}
	s.log.Printf("server %s listening on %s, data directory %s", Version, addr, dataDir)
	if exposed {
		s.log.Print(exposedWarning)
		fmt.Fprintln(os.Stderr, exposedWarning)
	}
	fmt.Printf("ReelAnvil %s: http://%s\nData directory: %s\n", Version, displayAddr(listener.Addr()), dataDir)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		select {
		case <-ctx.Done():
		case <-s.restart:
		}
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(shutdown)
	}()
	err = server.Serve(listener)
	defer s.stopBackground()
	if !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	// Let in-flight requests (such as the restart page) finish before the
	// deferred database and log closes run.
	<-shutdownDone
	select {
	case <-s.restart:
		s.log.Print("restarting")
		return ErrRestart
	default:
		return nil
	}
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
