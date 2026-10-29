package app

import (
	"bytes"
	"embed"
	"encoding/base64"
	"encoding/json"
	"html/template"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/gkuba/ReelAnvil/internal/store"
)

// The web UI is embedded so the binary is self-contained and never fetches
// remote assets.
//
//go:embed web
var webAssets embed.FS

// pages maps a page name to its template set: the shared layout and sidebar
// plus web/templates/<name>.html, which defines "content".
var pages = parsePages("overview", "campaigns", "general", "themes", "connectors", "storage", "diagnostics", "placeholder")

func templateFuncs() template.FuncMap {
	return template.FuncMap{
		"bodyTheme":     bodyTheme,
		"platformName":  platformName,
		"deadlineLabel": deadlineLabel,
		"money":         store.FormatMoney,
		"moneyInput":    store.FormatMoneyInput,
		"isManual":      func(c store.Campaign) bool { return c.SourceKind == "manual" },
		"hasValue":      func(v string) bool { return strings.TrimSpace(v) != "" },
	}
}

func parsePages(names ...string) map[string]*template.Template {
	out := make(map[string]*template.Template, len(names))
	for _, name := range names {
		out[name] = template.Must(template.New(name).Funcs(templateFuncs()).ParseFS(webAssets,
			"web/templates/layout.html", "web/templates/sidebar.html", "web/templates/"+name+".html"))
	}
	return out
}

// pageBase holds the fields the shared layout and sidebar need. Every page's
// data struct embeds it.
type pageBase struct {
	Title, Section, BodyTheme, IconURL string
	Version, DataDir, Address          string
	Flash                              *flash
}

func (s *Server) base(w http.ResponseWriter, r *http.Request, title, section string) pageBase {
	s.mu.Lock()
	cfg := s.cfg
	s.mu.Unlock()
	return pageBase{
		Title: title, Section: section, BodyTheme: bodyTheme(cfg.Theme),
		IconURL: iconURL(selectedIcon(cfg), 64), Version: Version,
		DataDir: s.dataDir, Address: s.addr, Flash: takeFlash(w, r),
	}
}

// render executes a page into a buffer first, so a template error produces a
// clean 500 instead of a half-written 200 page.
func (s *Server) render(w http.ResponseWriter, name string, data any) {
	var buf bytes.Buffer
	if err := pages[name].ExecuteTemplate(&buf, "layout", data); err != nil {
		s.log.Printf("render %s: %v", name, err)
		http.Error(w, "page rendering failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = buf.WriteTo(w)
}

// flash is a one-time message carried across a POST/redirect/GET in a
// short-lived cookie, so messages cannot be injected through link parameters.
type flash struct {
	Text  string `json:"t"`
	Error bool   `json:"e,omitempty"`
}

const flashCookie = "reelanvil_flash"

func setFlash(w http.ResponseWriter, text string, isError bool) {
	b, err := json.Marshal(flash{Text: text, Error: isError})
	if err != nil {
		return
	}
	http.SetCookie(w, &http.Cookie{Name: flashCookie, Value: base64.RawURLEncoding.EncodeToString(b), Path: "/", MaxAge: 60, HttpOnly: true, SameSite: http.SameSiteStrictMode})
}

func takeFlash(w http.ResponseWriter, r *http.Request) *flash {
	c, err := r.Cookie(flashCookie)
	if err != nil {
		return nil
	}
	http.SetCookie(w, &http.Cookie{Name: flashCookie, Path: "/", MaxAge: -1, Expires: time.Unix(0, 0), HttpOnly: true, SameSite: http.SameSiteStrictMode})
	b, err := base64.RawURLEncoding.DecodeString(c.Value)
	if err != nil {
		return nil
	}
	var f flash
	if json.Unmarshal(b, &f) != nil || f.Text == "" || len(f.Text) > 500 {
		return nil
	}
	return &f
}

// staticHandler serves web/static at /assets/ without directory listings.
func staticHandler() http.Handler {
	sub, err := fs.Sub(webAssets, "web/static")
	if err != nil {
		panic(err)
	}
	files := http.StripPrefix("/assets/", http.FileServer(http.FS(sub)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/") {
			http.NotFound(w, r)
			return
		}
		// Asset URLs carry ?v=<version>; revalidate in case of dev rebuilds.
		w.Header().Set("Cache-Control", "no-cache")
		files.ServeHTTP(w, r)
	})
}

// sections maps placeholder and settings URLs to their page and title.
var sections = map[string]struct{ page, title string }{
	"/jobs":                {"placeholder", "Jobs"},
	"/media":               {"placeholder", "Media"},
	"/review":              {"placeholder", "Review"},
	"/services":            {"placeholder", "Services"},
	"/diagnostics":         {"diagnostics", "Diagnostics"},
	"/settings":            {"general", "Settings"},
	"/settings/themes":     {"themes", "Themes"},
	"/settings/connectors": {"connectors", "Connectors"},
	"/settings/storage":    {"storage", "Paths & Storage"},
}

type settingsPageData struct {
	pageBase
	Config      Settings
	Uptime      string
	Themes      []Theme
	AllThemes   []Theme
	EditID      string
	EditTheme   Theme
	EditColors  []ColorField
	ThemeError  string
	IconOptions []IconOption
}

func (s *Server) page(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.URL.Path == "/" {
		s.overviewPage(w, r)
		return
	}
	sec, ok := sections[r.URL.Path]
	if !ok {
		http.NotFound(w, r)
		return
	}
	// The sidebar highlights by section ID: the last path element.
	section := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
	data := settingsPageData{pageBase: s.base(w, r, sec.title, section)}
	s.mu.Lock()
	data.Config = s.cfg
	data.Themes = sortedThemes(s.themes)
	data.AllThemes = append(builtinOptions(), data.Themes...)
	data.ThemeError = s.themeError
	data.EditID = r.URL.Query().Get("edit")
	if _, ok := s.themes[data.EditID]; !ok {
		data.EditID = ""
		if len(data.Themes) > 0 {
			data.EditID = data.Themes[0].ID
		}
	}
	data.EditTheme = s.themes[data.EditID]
	data.IconOptions = iconOptions(selectedIcon(s.cfg))
	s.mu.Unlock()
	data.EditColors = colorFields(data.EditTheme)
	data.Uptime = time.Since(s.started).Truncate(time.Second).String()
	s.render(w, sec.page, data)
}
