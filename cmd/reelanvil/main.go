package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/gkuba/ReelAnvil/internal/app"
)

const usage = `usage: reelanvil <command> [flags]

Commands:
  serve     run the local web interface
  doctor    print local diagnostics without changing files
  version   print the build version

Flags for serve and doctor:
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
		run = app.Doctor
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
	if err := run(opts); err != nil {
		fmt.Fprintln(os.Stderr, "reelanvil:", err)
		os.Exit(1)
	}
}

// newFlags binds the serve/doctor flags to opts. Environment variables provide
// the defaults so existing REELANVIL_ADDR usage keeps working.
func newFlags(command string, opts *app.Options) *flag.FlagSet {
	flags := flag.NewFlagSet("reelanvil "+command, flag.ContinueOnError)
	flags.StringVar(&opts.DataDir, "data-dir", envOr("REELANVIL_DATA_DIR", "."), "directory holding settings/, data/ and logs/ (env REELANVIL_DATA_DIR)")
	flags.StringVar(&opts.Addr, "addr", envOr("REELANVIL_ADDR", app.DefaultAddr), "loopback address to listen on (env REELANVIL_ADDR)")
	return flags
}

func printUsage(w io.Writer) {
	fmt.Fprint(w, usage)
	flags := newFlags("serve", &app.Options{})
	flags.SetOutput(w)
	flags.PrintDefaults()
}

func envOr(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}
