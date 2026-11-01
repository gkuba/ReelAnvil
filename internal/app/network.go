package app

// Network settings: where the server listens, which extra Host names it
// serves, and restarting the process so a changed address takes effect.

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// ErrRestart is returned by Serve after a restart was requested from the UI.
// The caller re-executes the binary (see Reexec) once Serve has cleaned up.
var ErrRestart = errors.New("restart requested")

const maxAllowedHosts = 32

// restartDelay is how long the restart page's assets have to load.
var restartDelay = 1500 * time.Millisecond

var hostNamePattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)*$`)

// parseAllowedHosts splits a comma, space or newline separated list of host
// names, normalising and de-duplicating them. IP addresses are refused because
// an exposed server always accepts them.
func parseAllowedHosts(v string) ([]string, error) {
	var out []string
	for _, field := range strings.FieldsFunc(v, func(r rune) bool { return r == ',' || unicode.IsSpace(r) }) {
		h := normalizeHost(field)
		if net.ParseIP(strings.Trim(h, "[]")) != nil {
			return nil, fmt.Errorf("%q is an IP address; IP addresses are always allowed", field)
		}
		if len(h) > 253 || !hostNamePattern.MatchString(h) {
			return nil, fmt.Errorf("%q is not a valid host name", field)
		}
		if !slices.Contains(out, h) {
			out = append(out, h)
		}
	}
	if len(out) > maxAllowedHosts {
		return nil, fmt.Errorf("at most %d host names are allowed", maxAllowedHosts)
	}
	return out, nil
}

// listenAddress builds host:port from the Network form. The bind address must
// be an IP literal or localhost, so saving never depends on DNS.
func listenAddress(bind, port string) (string, error) {
	bind = strings.Trim(strings.TrimSpace(bind), "[]")
	if bind == "" {
		bind = "0.0.0.0"
	}
	if bind != "localhost" && net.ParseIP(bind) == nil {
		return "", errors.New("bind address must be an IP address such as 0.0.0.0 or 127.0.0.1")
	}
	n, err := strconv.Atoi(strings.TrimSpace(port))
	if err != nil || n < 1 || n > 65535 {
		return "", errors.New("port must be a number from 1 to 65535")
	}
	return net.JoinHostPort(bind, strconv.Itoa(n)), nil
}

// validListenAddress checks a saved listen address.
func validListenAddress(addr string) error {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	_, err = listenAddress(host, port)
	return err
}

// networkView is what the Network card on Settings > General shows.
type networkView struct {
	Running, Bind, Port, Hosts, Hostname string
	AddrOverride, HostsOverride, Exposed bool
}

// network describes the saved and running network settings. Callers hold s.mu.
func (s *Server) network() networkView {
	v := networkView{Running: s.addr, Hosts: strings.Join(s.cfg.AllowedHosts, "\n"),
		AddrOverride: s.addrOverride, HostsOverride: s.hostsOverride, Exposed: s.hosts.exposed}
	v.Bind, v.Port, _ = net.SplitHostPort(s.cfg.ListenAddress)
	v.Hostname, _ = os.Hostname()
	return v
}

// restartPending reports whether saved network settings differ from the ones
// in use. Values set on the command line are not pending. Callers hold s.mu.
func (s *Server) restartPending() bool {
	return (!s.addrOverride && s.cfg.ListenAddress != s.addr) ||
		(!s.hostsOverride && !slices.Equal(s.cfg.AllowedHosts, s.runHosts))
}

// nextAddr is the address the process will listen on after a restart.
// Callers hold s.mu.
func (s *Server) nextAddr() string {
	if s.addrOverride {
		return s.addr
	}
	return s.cfg.ListenAddress
}

func (s *Server) saveNetwork(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	addr, err := listenAddress(r.FormValue("bind_address"), r.FormValue("port"))
	if err == nil {
		var hosts []string
		if hosts, err = parseAllowedHosts(r.FormValue("allowed_hosts")); err == nil {
			err = s.storeNetwork(addr, hosts)
		}
	}
	if err != nil {
		setFlash(w, "Network settings not saved: "+err.Error()+".", true)
		http.Redirect(w, r, "/settings", http.StatusSeeOther)
		return
	}
	msg := "Network settings saved."
	s.mu.Lock()
	if s.restartPending() {
		msg += " Restart ReelAnvil to apply them."
	}
	s.mu.Unlock()
	if exposed, _ := exposedAddress(addr); !exposed && !remoteIsLoopback(r) {
		msg += " After the restart ReelAnvil will only be reachable from its own machine."
	}
	setFlash(w, msg, false)
	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}

func (s *Server) storeNetwork(addr string, hosts []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := s.cfg
	next.ListenAddress, next.AllowedHosts = addr, hosts
	if err := save(s.cfgPath, next); err != nil {
		s.log.Println("settings save failed:", err)
		return errors.New("could not write settings")
	}
	s.cfg = next
	return nil
}

func remoteIsLoopback(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	ip := net.ParseIP(host)
	return err == nil && ip != nil && ip.IsLoopback()
}

type restartPageData struct {
	pageBase
	RedirectURL string
}

func (s *Server) restartHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.mu.Lock()
	target, current := s.nextAddr(), s.addr
	s.mu.Unlock()
	// A new address that cannot be bound would leave nothing running, so
	// check it while the current server is still up.
	if target != current {
		l, err := net.Listen("tcp", target)
		if err != nil {
			setFlash(w, "ReelAnvil was not restarted: cannot listen on "+target+": "+err.Error()+".", true)
			http.Redirect(w, r, "/settings", http.StatusSeeOther)
			return
		}
		l.Close()
	}
	s.log.Printf("restart requested from %s, next address %s", r.RemoteAddr, target)
	data := restartPageData{pageBase: s.base(w, r, "Restarting", "settings"), RedirectURL: restartURL(r.Host, target)}
	data.RestartPending = false
	s.render(w, "restart", data)
	// Keep serving briefly so the browser can load the page's stylesheet,
	// script and icons before the listener closes.
	time.AfterFunc(restartDelay, s.requestRestart)
}

// restartURL is where the browser goes once the server is back: the host it
// used now (the new address's IP when it binds to one interface) and the new port.
func restartURL(requestHost, target string) string {
	host, _, err := net.SplitHostPort(requestHost)
	if err != nil {
		host = strings.Trim(requestHost, "[]")
	}
	bind, port, _ := net.SplitHostPort(target)
	if ip := net.ParseIP(bind); bind != "localhost" && ip != nil && !ip.IsUnspecified() {
		current := net.ParseIP(host)
		if !ip.IsLoopback() || current == nil || !current.IsLoopback() {
			host = bind
		}
	}
	return "http://" + net.JoinHostPort(host, port) + "/settings"
}

func (s *Server) requestRestart() {
	if s.restart != nil {
		s.restartOnce.Do(func() { close(s.restart) })
	}
}
