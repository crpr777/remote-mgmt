//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

// hideWindowsConsole hides the console window for a command
func hideWindowsConsole(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
}
