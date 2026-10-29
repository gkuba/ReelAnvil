package main

import (
	"fmt"
	"os"

	"github.com/gkuba/ReelAnvil/internal/app"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: reelanvil serve | doctor")
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "serve":
		err = app.Serve()
	case "doctor":
		err = app.Doctor()
	default:
		fmt.Fprintln(os.Stderr, "unknown command:", os.Args[1])
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "reelanvil:", err)
		os.Exit(1)
	}
}
