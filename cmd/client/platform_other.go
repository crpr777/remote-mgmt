//go:build !windows

package main

import "os/exec"

// hideWindowsConsole is a no-op on non-Windows platforms
func hideWindowsConsole(cmd *exec.Cmd) {
	// No-op on non-Windows
}
