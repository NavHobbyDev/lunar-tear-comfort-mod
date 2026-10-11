//go:build !windows

package main

import (
	"os"

	"golang.org/x/term"
)

func colorSupported() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}
