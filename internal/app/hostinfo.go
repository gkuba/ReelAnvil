package app

// Host facts for the "Pull a model" card: how much RAM this machine has, which
// GPU vendors it lists, and whether Ollama is running a model on a GPU. Linux
// only; anything that cannot be read is reported as "not detected".

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	procMeminfo  = "/proc/meminfo"
	drmVendorGlb = "/sys/class/drm/card*/device/vendor"
)

// hostInfo is what the AI page shows about this machine.
type hostInfo struct {
	RAMKB int64    // 0 when not detected
	GPUs  []string // vendor names, e.g. "AMD"; empty when none listed
}

func (h hostInfo) RAMText() string {
	if h.RAMKB <= 0 {
		return "not detected"
	}
	return fmt.Sprintf("%.1f GB", float64(h.RAMKB)/1024/1024)
}

func (h hostInfo) GPUText() string {
	if len(h.GPUs) == 0 {
		return "not detected"
	}
	return strings.Join(h.GPUs, ", ")
}

// parseMemTotalKB reads MemTotal (in kB) from /proc/meminfo text.
func parseMemTotalKB(path string) int64 {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) >= 2 && fields[0] == "MemTotal:" {
			n, err := strconv.ParseInt(fields[1], 10, 64)
			if err != nil || n < 0 {
				return 0
			}
			return n
		}
	}
	return 0
}

// gpuVendorName maps a PCI vendor id such as "0x10de" to a name.
func gpuVendorName(id string) string {
	switch strings.ToLower(strings.TrimSpace(id)) {
	case "0x1002":
		return "AMD"
	case "0x10de":
		return "NVIDIA"
	case "0x8086":
		return "Intel"
	}
	return "unknown"
}

// detectHost reads RAM from meminfoPath and GPU vendors from files matching
// vendorGlob. Both are parameters so tests can point them at fixtures.
func detectHost(meminfoPath, vendorGlob string) hostInfo {
	h := hostInfo{RAMKB: parseMemTotalKB(meminfoPath)}
	files, _ := filepath.Glob(vendorGlob)
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		h.GPUs = append(h.GPUs, gpuVendorName(string(b)))
	}
	return h
}

// ollamaCompute says whether Ollama runs its loaded models on a GPU or CPU,
// from /api/ps.
func ollamaCompute(addr string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var ps struct {
		Models []struct {
			SizeVRAM int64 `json:"size_vram"`
		} `json:"models"`
	}
	if err := ollamaGet(ctx, addr, "/api/ps", &ps); err != nil {
		return "Ollama not reachable"
	}
	if len(ps.Models) == 0 {
		return "not known until a model is loaded"
	}
	for _, m := range ps.Models {
		if m.SizeVRAM > 0 {
			return "GPU"
		}
	}
	return "CPU"
}

// ---- catalog ----

type catalogModel struct {
	Name, Download, Note string
	NeedGB               int
}

var modelCatalog = []catalogModel{
	{"qwen2.5:3b", "1.9 GB", "fast, weaker picks", 8},
	{"qwen2.5:7b", "4.7 GB", "good balance", 12},
	{"qwen2.5:14b", "9.0 GB", "best picks, slow on CPU", 20},
}

// modelChoice is one option in the dropdown.
type modelChoice struct {
	Name, Label string
	Selected    bool
}

// modelChoices labels the catalog for ramKB (0 = unknown, so nothing is
// recommended). The recommended model is the largest that fits; it is the
// default selection (with no recommendation, the first option).
func modelChoices(ramKB int64, installed []string) []modelChoice {
	have := map[string]bool{}
	for _, n := range installed {
		have[n] = true
	}
	rec := -1
	if ramKB > 0 {
		for i, m := range modelCatalog {
			if int64(m.NeedGB)*1024*1024 <= ramKB {
				rec = i
			}
		}
	}
	var out []modelChoice
	for i, m := range modelCatalog {
		tag := ""
		switch {
		case i == rec:
			tag = "Recommended"
		case ramKB > 0 && int64(m.NeedGB)*1024*1024 > ramKB:
			tag = "Needs more RAM"
		case ramKB > 0:
			tag = "Fits"
		}
		label := fmt.Sprintf("%s — %s — needs %d GB", m.Name, m.Download, m.NeedGB)
		if tag != "" {
			label += " — " + tag
		}
		if have[m.Name] {
			label += " — Installed"
		}
		label += " (" + m.Note + ")"
		out = append(out, modelChoice{Name: m.Name, Label: label, Selected: i == rec})
	}
	if rec < 0 && len(out) > 0 {
		out[0].Selected = true
	}
	return out
}
