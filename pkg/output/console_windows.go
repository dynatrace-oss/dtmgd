//go:build windows

package output

import (
	"os"

	"golang.org/x/sys/windows"
)

// enableVT switches on virtual-terminal processing for a Windows console, so
// it renders escape codes instead of printing them, and reports whether that
// worked. Windows 10 and later support it, but only once a program asks; see
// https://learn.microsoft.com/en-us/windows/console/console-virtual-terminal-sequences.
// Adapted from dtctl's pkg/output/console_windows.go.
func enableVT(f *os.File) bool {
	handle := windows.Handle(f.Fd())
	var mode uint32
	if err := windows.GetConsoleMode(handle, &mode); err != nil {
		return false
	}
	if mode&windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING != 0 {
		return true
	}
	return windows.SetConsoleMode(handle, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING) == nil
}
