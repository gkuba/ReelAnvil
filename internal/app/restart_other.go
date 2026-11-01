//go:build !unix

package app

import "errors"

// Reexec is only supported on Unix; elsewhere the user starts ReelAnvil again.
func Reexec() error {
	return errors.New("restart is not supported on this platform; start ReelAnvil again")
}
