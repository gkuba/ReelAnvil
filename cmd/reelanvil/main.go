package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/gkuba/ReelAnvil/internal/app"
)

const usage = `usage: reelanvil <command> [flags]

Commands:
  serve     run the local web interface
  doctor    print diagnostics; with --repair [--clean] fix a system install
  install   set up Podman, the reelanvil user and /opt/ReelAnvil (--service adds systemd units)
  upgrade   update the binary, compose.yaml and images of a system install (--dry-run)
  uninstall remove what install set up (--yes skips the questions; --delete-data also deletes settings and media)
  worker    run media operations for a local serve (--data-dir, --server 127.0.0.1:4747)
  version   print the build version

Flags for serve and doctor (install, upgrade and uninstall take --yes, --dir, --port):
`

func main() {
	if len(os.Args) < 2 {
		printUsage(os.Stderr)
		os.Exit(2)
	}
	command := os.Args[1]
	var run func(app.Options) error
	switch command {
	case "serve":
		run = app.Serve
	case "doctor":
		os.Exit(runDoctor(os.Args[2:]))
	case "worker":
		os.Exit(runWorker(os.Args[2:]))
	case "install", "upgrade", "uninstall":
		os.Exit(runInstall(command, os.Args[2:]))
	case "version", "--version":
		fmt.Println("reelanvil", app.Version)
		return
	case "help", "-h", "--help":
		printUsage(os.Stdout)
		return
	default:
		fmt.Fprintln(os.Stderr, "unknown command:", command)
		printUsage(os.Stderr)
		os.Exit(2)
	}
	var opts app.Options
	flags := newFlags(command, &opts)
	if err := flags.Parse(os.Args[2:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		os.Exit(2)
	}
	if flags.NArg() > 0 {
		fmt.Fprintln(os.Stderr, "unexpected arguments:", flags.Args())
		os.Exit(2)
	}
	err := run(opts)
	if errors.Is(err, app.ErrRestart) {
		err = app.Reexec()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "reelanvil:", err)
		os.Exit(1)
	}
}

// newFlags binds the serve/doctor flags to opts. Environment variables provide
// the defaults so existing REELANVIL_ADDR usage keeps working.
func newFlags(command string, opts *app.Options) *flag.FlagSet {
	flags := flag.NewFlagSet("reelanvil "+command, flag.ContinueOnError)
	flags.StringVar(&opts.DataDir, "data-dir", envOr("REELANVIL_DATA_DIR", "."), "directory holding settings/, data/ and logs/ (env REELANVIL_DATA_DIR)")
	flags.StringVar(&opts.Addr, "addr", os.Getenv("REELANVIL_ADDR"), "host:port to listen on for this run, overriding Settings > General (default "+app.DefaultAddr+"; env REELANVIL_ADDR)")
	flags.Func("allowed-hosts", "comma-separated extra host names for this run, overriding Settings > General (env REELANVIL_ALLOWED_HOSTS)", func(v string) error {
		opts.AllowedHosts = append([]string{}, splitList(v)...)
		return nil
	})
	if v, ok := os.LookupEnv("REELANVIL_ALLOWED_HOSTS"); ok {
		opts.AllowedHosts = append([]string{}, splitList(v)...)
	}
	return flags
}

func printUsage(w io.Writer) {
	fmt.Fprint(w, usage)
	flags := newFlags("serve", &app.Options{})
	flags.SetOutput(w)
	flags.PrintDefaults()
}

func splitList(v string) []string {
	var out []string
	for _, part := range strings.Split(v, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func envOr(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

func runWorker(args []string) int {
	var opts app.WorkerOptions
	flags := flag.NewFlagSet("reelanvil worker", flag.ContinueOnError)
	flags.StringVar(&opts.DataDir, "data-dir", envOr("REELANVIL_DATA_DIR", "."), "the serve data directory (env REELANVIL_DATA_DIR)")
	flags.StringVar(&opts.Server, "server", "127.0.0.1:4747", "loopback host:port of the web app")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() > 0 {
		fmt.Fprintln(os.Stderr, "unexpected arguments:", flags.Args())
		return 2
	}
	if err := app.RunWorker(opts); err != nil {
		fmt.Fprintln(os.Stderr, "reelanvil worker:", err)
		return 1
	}
	return 0
}
