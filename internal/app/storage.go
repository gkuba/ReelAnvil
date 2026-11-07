package app

// Paths & Storage: the media root with its logical sub-folders, the storage
// limit, and the check that reports whether the root can be used. The server
// only creates missing folders; it never deletes or moves anything.

import (
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Logical storage areas, by semantic key. The folder names below are the only
// place they are spelled out; business logic uses the keys.
type folderKey string

const (
	FolderIncoming  folderKey = "incoming"
	FolderTemp      folderKey = "temp"
	FolderCompleted folderKey = "completed"
	FolderFailed    folderKey = "failed"
	FolderArchive   folderKey = "archive"
)

// incomingFolderName is the arrival folder's name on disk (candidates were
// incoming/ and inbox/; the maintainer's default is incoming).
const incomingFolderName = "incoming"

// storageFolders lists the areas in display order.
var storageFolders = []struct {
	Key  folderKey
	Name string
	Desc string
}{
	{FolderIncoming, incomingFolderName, "Manual drops and finished downloads waiting to be picked up."},
	{FolderTemp, "temp", "Working space for downloads, transcription, analysis and rendering."},
	{FolderCompleted, "completed", "Finished clips waiting for review and export."},
	{FolderFailed, "failed", "Artifacts kept from jobs that failed."},
	{FolderArchive, "archive", "Preserved original source media."},
}

const (
	maxMediaRoot      = 512
	maxStorageLimitGB = 1 << 20
	folderMode        = 0o750
)

// Bounds for measuring used space, so a huge tree cannot hang a request.
var (
	usedWalkBudget  = 2 * time.Second
	usedWalkEntries = 200_000
)

// mediaRootChars is the same set the installer accepts for the container mount.
var mediaRootChars = regexp.MustCompile(`^[A-Za-z0-9._/-]+$`)

// normalizeMediaRoot checks a media root typed in the form and returns it
// cleaned. It must be an absolute path without "..", not the filesystem root
// and not inside a system location.
func normalizeMediaRoot(v string) (string, error) {
	v = strings.TrimSpace(v)
	switch {
	case v == "":
		return "", errors.New("media root is required")
	case len(v) > maxMediaRoot || strings.IndexFunc(v, unicode.IsControl) >= 0:
		return "", errors.New("media root must be one line of at most 512 characters")
	case !strings.HasPrefix(v, "/"):
		return "", errors.New("media root must be an absolute path starting with /")
	case !mediaRootChars.MatchString(v):
		return "", errors.New("media root may only contain letters, digits and . _ - /")
	}
	for _, el := range strings.Split(v, "/") {
		if el == ".." {
			return "", errors.New("media root must not contain a .. part")
		}
	}
	c := filepath.Clean(v)
	parts := strings.Split(c[1:], "/")
	top := parts[0]
	// These rules match the installer's check of the container mount.
	switch {
	case c == "/" || len(parts) < 2:
		return "", fmt.Errorf("%s is too close to the filesystem root; use a folder such as /srv/reelanvil/media", c)
	case strings.HasPrefix(top, "lib") || top == "usr" || top == "etc" || top == "bin" || top == "sbin" || top == "boot" ||
		top == "proc" || top == "sys" || top == "dev" || top == "run" || top == "root":
		return "", fmt.Errorf("%s is a system location", c)
	case (top == "var" || top == "home" || top == "tmp") && len(parts) <= 2:
		return "", fmt.Errorf("%s is a system location", c)
	case top == "var" && len(parts) >= 3 && parts[1] == "lib" && parts[2] == "containers":
		return "", fmt.Errorf("%s is a system location", c)
	}
	return c, nil
}

// parseStorageLimit reads the limit in GB; empty means 0 (no limit).
func parseStorageLimit(v string) (int, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 || n > maxStorageLimitGB {
		return 0, fmt.Errorf("storage limit must be a whole number of GB from 0 (no limit) to %d", maxStorageLimitGB)
	}
	return n, nil
}

// effectiveMediaRoot is the saved media root, or <data-dir>/media when none is saved.
func effectiveMediaRoot(c Settings, dataDir string) string {
	if c.MediaRoot != "" {
		return c.MediaRoot
	}
	return filepath.Join(dataDir, "media")
}

// folderPath is the path of one logical area under root.
func folderPath(root string, key folderKey) string {
	for _, f := range storageFolders {
		if f.Key == key {
			return filepath.Join(root, f.Name)
		}
	}
	return ""
}

// ensureMediaFolders creates the missing logical folders under an existing
// root and returns the names it created. Existing folders are left alone.
func ensureMediaFolders(root string) (created []string, err error) {
	fi, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !fi.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", root)
	}
	for _, f := range storageFolders {
		p := filepath.Join(root, f.Name)
		if _, serr := os.Lstat(p); serr == nil {
			continue
		}
		if err := os.Mkdir(p, folderMode); err != nil && !errors.Is(err, fs.ErrExist) {
			return created, err
		}
		created = append(created, f.Name)
	}
	return created, nil
}

// storageReport is the outcome of Check on a media root.
type storageReport struct {
	Root                   string
	Exists, IsDir          bool
	Writable               bool
	WriteErr               string
	FreeKnown              bool
	Free, Total            uint64
	Used                   int64
	UsedPartial, UsedKnown bool
	LimitGB                int
	Missing                []string // logical folders not present
}

func (r storageReport) limitBytes() int64 { return int64(r.LimitGB) << 30 }

// Over reports whether the measured use is above the limit. A partial
// measurement that is already over is over; one that is not proves nothing.
func (r storageReport) Over() bool { return r.LimitGB > 0 && r.UsedKnown && r.Used > r.limitBytes() }

// OK means the root is a writable directory and within its limit.
func (r storageReport) OK() bool { return r.IsDir && r.Writable && !r.Over() }

// checkStorage inspects root. It creates and removes one temporary file to
// test writing, and walks the tree (bounded) to measure used space.
func checkStorage(root string, limitGB int, withUsed bool) storageReport {
	r := storageReport{Root: root, LimitGB: limitGB}
	fi, err := os.Stat(root)
	if err != nil {
		return r
	}
	r.Exists, r.IsDir = true, fi.IsDir()
	if !r.IsDir {
		return r
	}
	r.Writable, r.WriteErr = writeTest(root)
	r.Free, r.Total, r.FreeKnown = diskSpace(root)
	for _, f := range storageFolders {
		if st, err := os.Stat(filepath.Join(root, f.Name)); err != nil || !st.IsDir() {
			r.Missing = append(r.Missing, f.Name)
		}
	}
	if withUsed {
		r.Used, r.UsedPartial = usedSpace(root)
		r.UsedKnown = true
	}
	return r
}

// writeTest creates and removes a temporary file in dir.
func writeTest(dir string) (bool, string) {
	f, err := os.CreateTemp(dir, ".reelanvil-write-test-*")
	if err != nil {
		return false, err.Error()
	}
	name := f.Name()
	_, werr := f.Write([]byte("x"))
	cerr := f.Close()
	rerr := os.Remove(name)
	for _, e := range []error{werr, cerr, rerr} {
		if e != nil {
			return false, e.Error()
		}
	}
	return true, ""
}

// usedSpace adds up file sizes under root without following links. It stops
// after usedWalkEntries entries or usedWalkBudget and then reports partial.
func usedSpace(root string) (total int64, partial bool) {
	deadline := time.Now().Add(usedWalkBudget)
	n := 0
	errStop := errors.New("stop")
	_ = filepath.WalkDir(root, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable entries are skipped
		}
		if n++; n > usedWalkEntries || (n%256 == 0 && time.Now().After(deadline)) {
			partial = true
			return errStop
		}
		if d.Type().IsRegular() {
			if info, ierr := d.Info(); ierr == nil {
				total += info.Size()
			}
		}
		return nil
	})
	return total, partial
}

// humanBytes formats a size with binary units, labelled KB/MB/GB.
func humanBytes(n int64) string {
	if n < 0 {
		n = 0
	}
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit && exp < 4; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTP"[exp])
}

// summary is the one-paragraph result shown after Check.
func (r storageReport) summary() (string, bool) {
	switch {
	case !r.Exists:
		return fmt.Sprintf("Media root %s does not exist. Create it with write access for the service user (an installed host does this on `sudo reelanvil upgrade`), then save the settings to create the sub-folders.", r.Root), true
	case !r.IsDir:
		return fmt.Sprintf("Media root %s is not a directory.", r.Root), true
	}
	var parts []string
	bad := false
	if r.Writable {
		parts = append(parts, "writable")
	} else {
		parts = append(parts, "NOT writable ("+clip(r.WriteErr, 120)+")")
		bad = true
	}
	if r.FreeKnown {
		parts = append(parts, fmt.Sprintf("%s free of %s", humanBytes(int64(r.Free)), humanBytes(int64(r.Total))))
	}
	if r.UsedKnown {
		u := "ReelAnvil data under it: " + humanBytes(r.Used)
		if r.UsedPartial {
			u = "At least " + humanBytes(r.Used) + " under it (measuring stopped early)"
		}
		if r.LimitGB > 0 {
			u += fmt.Sprintf(" of a %d GB limit", r.LimitGB)
		} else {
			u += ", no limit set"
		}
		parts = append(parts, u)
	}
	msg := fmt.Sprintf("Media root %s: %s.", r.Root, strings.Join(parts, "; "))
	if r.Over() {
		msg += " Over the storage limit: free space or raise the limit before new jobs can run."
		bad = true
	}
	if len(r.Missing) > 0 {
		msg += " Missing folders: " + strings.Join(r.Missing, ", ") + " (saving the settings creates them)."
	}
	return msg, bad
}

// ---- handlers ----

func (s *Server) storageSettings(w http.ResponseWriter, r *http.Request) {
	s.serviceSaveThen("/settings/storage", "Storage settings", func(r *http.Request) (func(*Settings), error) {
		root, err := normalizeMediaRoot(r.FormValue("media_root"))
		if err != nil {
			return nil, err
		}
		limit, err := parseStorageLimit(r.FormValue("storage_limit_gb"))
		if err != nil {
			return nil, err
		}
		return func(c *Settings) { c.MediaRoot, c.StorageLimitGB = root, limit }, nil
	}, func(c Settings) string {
		root := effectiveMediaRoot(c, s.dataDir)
		created, err := ensureMediaFolders(root)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			return " The media root does not exist yet, so no sub-folders were created. Create it, or run `sudo reelanvil upgrade` on an installed host."
		case err != nil:
			return " Sub-folders could not be created: " + clip(err.Error(), 120) + "."
		case len(created) > 0:
			return " Created " + strings.Join(created, ", ") + "."
		}
		return ""
	})(w, r)
}

func (s *Server) storageCheck(w http.ResponseWriter, r *http.Request) {
	s.serviceTest("/settings/storage", func(c Settings) (string, bool) {
		return checkStorage(effectiveMediaRoot(c, s.dataDir), c.StorageLimitGB, true).summary()
	})(w, r)
}

type storageFolderView struct {
	Name, Desc, Path string
	Present          bool
}

type storagePageData struct {
	pageBase
	Config  Settings
	Root    string
	Default bool
	Folders []storageFolderView
	RootOK  bool
}

func (s *Server) storagePage(w http.ResponseWriter, r *http.Request, sec pageSpec, section string) {
	data := storagePageData{pageBase: s.base(w, r, sec.title, section)}
	s.mu.Lock()
	data.Config = s.cfg
	s.mu.Unlock()
	data.Root = effectiveMediaRoot(data.Config, s.dataDir)
	data.Default = data.Config.MediaRoot == ""
	if fi, err := os.Stat(data.Root); err == nil && fi.IsDir() {
		data.RootOK = true
	}
	for _, f := range storageFolders {
		p := filepath.Join(data.Root, f.Name)
		fi, err := os.Stat(p)
		data.Folders = append(data.Folders, storageFolderView{Name: f.Name + "/", Desc: f.Desc, Path: p, Present: err == nil && fi.IsDir()})
	}
	s.render(w, sec.page, data)
}

// MediaFolderNames lists the sub-folders of the media root in display order.
// The installer creates the same folders; a test keeps the two lists equal.
func MediaFolderNames() []string {
	out := make([]string, len(storageFolders))
	for k, f := range storageFolders {
		out[k] = f.Name
	}
	return out
}

// CheckMediaRoot reports whether v is accepted as a media root as typed. The
// installer applies the same rules to the folder it mounts into the media
// container; a test keeps the two in step.
func CheckMediaRoot(v string) error {
	c, err := normalizeMediaRoot(v)
	if err == nil && c != v {
		return errors.New("path is not clean")
	}
	return err
}
