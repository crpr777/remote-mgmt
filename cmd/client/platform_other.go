//go:build !windows

package main

import (
	"os/exec"
	"runtime"
)

// hideWindowsConsole is a no-op on non-Windows platforms
func hideWindowsConsole(cmd *exec.Cmd) {
	// No-op on non-Windows
}

// openBrowser opens a URL in the default browser
func openBrowser(url string) {
	switch runtime.GOOS {
	case "darwin":
		exec.Command("open", url).Start()
	default:
		exec.Command("xdg-open", url).Start()
	}
}
