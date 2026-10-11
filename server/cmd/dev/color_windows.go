//go:build windows

package main

import (
	"os"

	"golang.org/x/sys/windows"
	"golang.org/x/term"
)

func colorSupported() bool {
	fd := os.Stdout.Fd()
	if !term.IsTerminal(int(fd)) {
		return false
	}
	var mode uint32
	if windows.GetConsoleMode(windows.Handle(fd), &mode) != nil {
		return false
	}
	return windows.SetConsoleMode(windows.Handle(fd), mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING) == nil
}
