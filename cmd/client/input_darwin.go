//go:build darwin

package main

import (
	"fmt"
	"os/exec"
	"strings"
)

// mouseAction is the unified interface called by main.go
func mouseAction(input MouseInput) error {
	// Try cliclick first (faster, more reliable if installed)
	if _, err := exec.LookPath("cliclick"); err == nil {
		return cliclickMouseAction(input)
	}
	// Fallback to AppleScript
	return applescriptMouseAction(input)
}

func cliclickMouseAction(input MouseInput) error {
	switch input.Action {
	case "move":
		return exec.Command("cliclick", fmt.Sprintf("m:%d,%d", input.X, input.Y)).Run()
	case "click":
		return exec.Command("cliclick", fmt.Sprintf("c:%d,%d", input.X, input.Y)).Run()
	case "doubleclick":
		return exec.Command("cliclick", fmt.Sprintf("dc:%d,%d", input.X, input.Y)).Run()
	case "rightclick":
		return exec.Command("cliclick", fmt.Sprintf("rc:%d,%d", input.X, input.Y)).Run()
	default:
		return fmt.Errorf("unknown action: %s", input.Action)
	}
}

func applescriptMouseAction(input MouseInput) error {
	var script string
	switch input.Action {
	case "click":
		script = fmt.Sprintf(`
			tell application "System Events"
				click at {%d, %d}
			end tell`, input.X, input.Y)
	case "move":
		// AppleScript doesn't have a direct move, use coreGraphics via osascript
		script = fmt.Sprintf(`
			use framework "CoreGraphics"
			current application's CGEventPost(current application's kCGHIDEventTap, current application's CGEventCreateMouseEvent(missing value, current application's kCGEventMouseMoved, {%d, %d}, 0))
		`, input.X, input.Y)
	default:
		return fmt.Errorf("unknown action: %s (AppleScript fallback)", input.Action)
	}
	return exec.Command("osascript", "-e", script).Run()
}

// keyboardAction is the unified interface called by main.go
func keyboardAction(input KeyboardInput) error {
	// Try cliclick first
	if _, err := exec.LookPath("cliclick"); err == nil {
		return cliclickKeyboardAction(input)
	}
	return applescriptKeyboardAction(input)
}

func cliclickKeyboardAction(input KeyboardInput) error {
	switch input.Action {
	case "type":
		// cliclick t: command types text
		return exec.Command("cliclick", fmt.Sprintf("t:%s", input.Text)).Run()
	case "keydown", "keyup", "hotkey":
		// Fall back to AppleScript for key events
		return applescriptKeyboardAction(input)
	default:
		return fmt.Errorf("unknown action: %s", input.Action)
	}
}

func applescriptKeyboardAction(input KeyboardInput) error {
	var script string
	switch input.Action {
	case "type":
		// Escape text for AppleScript
		escaped := strings.ReplaceAll(input.Text, `"`, `\"`)
		script = fmt.Sprintf(`tell application "System Events" to keystroke "%s"`, escaped)
	case "hotkey":
		// Build modifier string
		mods := ""
		for _, m := range input.Modifiers {
			switch m {
			case "ctrl", "control":
				mods += " using control down"
			case "alt", "option":
				mods += " using option down"
			case "shift":
				mods += " using shift down"
			case "cmd", "command", "meta":
				mods += " using command down"
			}
		}
		escaped := strings.ReplaceAll(input.Text, `"`, `\"`)
		script = fmt.Sprintf(`tell application "System Events" to keystroke "%s"%s`, escaped, mods)
	case "keydown":
		script = fmt.Sprintf(`tell application "System Events" to key down %s`, input.Key)
	case "keyup":
		script = fmt.Sprintf(`tell application "System Events" to key up %s`, input.Key)
	default:
		return fmt.Errorf("unknown action: %s", input.Action)
	}
	return exec.Command("osascript", "-e", script).Run()
}

