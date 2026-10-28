package app

import (
	"embed"
	"fmt"
	"io/fs"
	"net/http"
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

func selectedIcon(cfg Settings) string {
	if validIconVariant(cfg.IconVariant) {
		return cfg.IconVariant
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
