package app

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"path"
	"strings"
)

// Branding assets are local and embedded so the server never fetches artwork or icons.
//
//go:embed branding/*
var brandingAssets embed.FS

type IconOption struct {
	ID, Name, PreviewURL string
	Selected             bool
}
type ThemeIconChoice struct {
	ID, Name, Variant string
	Selected          bool
}

var iconVariants = []IconOption{
	{ID: "blue-violet", Name: "Blue-Violet Film Glow"},
	{ID: "amethyst", Name: "Amethyst Glow"},
	{ID: "molten-orange", Name: "Molten Orange"},
}

func validIconVariant(v string) bool {
	for _, option := range iconVariants {
		if option.ID == v {
			return true
		}
	}
	return false
}

func cloneThemeIcons(src map[string]string) map[string]string {
	out := make(map[string]string, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func ensureThemeIcons(cfg *Settings, custom map[string]Theme) {
	if cfg.ThemeIcons == nil {
		cfg.ThemeIcons = map[string]string{}
	}
	for _, theme := range builtinOptions() {
		if !validIconVariant(cfg.ThemeIcons[theme.ID]) {
			cfg.ThemeIcons[theme.ID] = "blue-violet"
		}
	}
	for id := range custom {
		if !validIconVariant(cfg.ThemeIcons[id]) {
			cfg.ThemeIcons[id] = "blue-violet"
		}
	}
}

func selectedIcon(cfg Settings) string {
	if validIconVariant(cfg.ThemeIcons[cfg.Theme]) {
		return cfg.ThemeIcons[cfg.Theme]
	}
	return "blue-violet"
}

func iconOptions(variant string) []IconOption {
	out := make([]IconOption, len(iconVariants))
	for i, option := range iconVariants {
		out[i] = option
		out[i].PreviewURL = fmt.Sprintf("/assets/branding/icon-%s-64.png", option.ID)
		out[i].Selected = option.ID == variant
	}
	return out
}

func themeIconChoices(cfg Settings, custom map[string]Theme, selected string) []ThemeIconChoice {
	choices := make([]ThemeIconChoice, 0, len(builtinOptions())+len(custom))
	for _, theme := range builtinOptions() {
		choices = append(choices, ThemeIconChoice{ID: theme.ID, Name: theme.Name, Variant: cfg.ThemeIcons[theme.ID], Selected: selected == theme.ID})
	}
	for _, theme := range sortedThemes(custom) {
		choices = append(choices, ThemeIconChoice{ID: theme.ID, Name: theme.Name, Variant: cfg.ThemeIcons[theme.ID], Selected: selected == theme.ID})
	}
	return choices
}

func (s *Server) saveThemeIcon(w http.ResponseWriter, r *http.Request) {
	if !postOnly(w, r) {
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	key, variant := strings.TrimSpace(r.FormValue("theme_key")), strings.TrimSpace(r.FormValue("icon_variant"))
	s.mu.Lock()
	defer s.mu.Unlock()
	if !validTheme(key) {
		if _, ok := s.themes[key]; !ok {
			http.Error(w, "invalid theme", http.StatusBadRequest)
			return
		}
	}
	if !validIconVariant(variant) {
		http.Error(w, "invalid icon variant", http.StatusBadRequest)
		return
	}
	next := s.cfg
	next.ThemeIcons = cloneThemeIcons(s.cfg.ThemeIcons)
	next.ThemeIcons[key] = variant
	next.Version = 2
	ensureThemeIcons(&next, s.themes)
	if err := save(s.cfgPath, next); err != nil {
		s.log.Println("theme icon save failed:", err)
		http.Error(w, "could not save theme icon", http.StatusInternalServerError)
		return
	}
	s.cfg = next
	http.Redirect(w, r, "/settings/themes?icon_theme="+url.QueryEscape(key)+"&saved=1", http.StatusSeeOther)
}

func brandingHandler() http.Handler {
	sub, err := fs.Sub(brandingAssets, "branding")
	if err != nil {
		panic(err)
	}
	return http.StripPrefix("/assets/branding/", http.FileServer(http.FS(sub)))
}

func (s *Server) favicon(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.mu.Lock()
	variant := selectedIcon(s.cfg)
	s.mu.Unlock()
	name := "branding/favicon-" + variant + ".ico"
	b, err := brandingAssets.ReadFile(path.Clean(name))
	if err != nil {
		http.Error(w, "favicon unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "image/vnd.microsoft.icon")
	w.Header().Set("Cache-Control", "no-cache, must-revalidate")
	if r.Method == http.MethodGet {
		_, _ = w.Write(b)
	}
}

func iconURL(variant string, size int) string {
	if !validIconVariant(variant) {
		variant = "blue-violet"
	}
	if size != 16 && size != 32 && size != 48 && size != 64 && size != 128 && size != 256 && size != 512 {
		size = 64
	}
	return "/assets/branding/icon-" + strings.TrimSpace(variant) + fmt.Sprintf("-%d.png", size)
}
