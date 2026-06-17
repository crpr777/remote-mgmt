//go:build darwin

package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// cliclickPaths are common install locations checked when cliclick isn't on PATH
// (LaunchAgents have a minimal PATH that may not include Homebrew dirs).
var cliclickPaths = []string{
	"/usr/local/bin/cliclick",
	"/opt/homebrew/bin/cliclick",
}

// findCliclick returns the path to cliclick, or "" if not found.
func findCliclick() string {
	if p, err := exec.LookPath("cliclick"); err == nil {
		return p
	}
	for _, p := range cliclickPaths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// mouseAction is the unified interface called by main.go
func mouseAction(input MouseInput) error {
	if p := findCliclick(); p != "" {
		return cliclickMouseAction(p, input)
	}
	return cgMouseAction(input)
}

func cliclickMouseAction(bin string, input MouseInput) error {
	switch input.Action {
	case "move":
		return exec.Command(bin, fmt.Sprintf("m:%d,%d", input.X, input.Y)).Run()
	case "click":
		return exec.Command(bin, fmt.Sprintf("c:%d,%d", input.X, input.Y)).Run()
	case "doubleclick":
		return exec.Command(bin, fmt.Sprintf("dc:%d,%d", input.X, input.Y)).Run()
	case "rightclick":
		return exec.Command(bin, fmt.Sprintf("rc:%d,%d", input.X, input.Y)).Run()
	default:
		return fmt.Errorf("unknown mouse action: %s", input.Action)
	}
}

// cgMouseAction uses CoreGraphics events via JXA (JavaScript for Automation)
// for mouse control. JXA has proper access to CG constants and functions
// unlike AppleScript's ObjC bridge which can't resolve plain C enums.
func cgMouseAction(input MouseInput) error {
	// CoreGraphics event type constants
	const (
		cgMouseMoved    = 5
		cgLeftMouseDown = 1
		cgLeftMouseUp   = 2
		cgRightMouseDown = 3
		cgRightMouseUp   = 4
	)

	post := func(eventType, button int) string {
		return fmt.Sprintf(
			"$.CGEventPost(0, $.CGEventCreateMouseEvent(null, %d, $.CGPointMake(%d, %d), %d));",
			eventType, input.X, input.Y, button)
	}

	var body string
	switch input.Action {
	case "move":
		body = post(cgMouseMoved, 0)
	case "click":
		body = post(cgLeftMouseDown, 0) + "\n" +
			"delay(0.02);\n" +
			post(cgLeftMouseUp, 0)
	case "doubleclick":
		body = post(cgLeftMouseDown, 0) + "\n" +
			post(cgLeftMouseUp, 0) + "\n" +
			"delay(0.05);\n" +
			post(cgLeftMouseDown, 0) + "\n" +
			post(cgLeftMouseUp, 0)
	case "rightclick":
		body = post(cgRightMouseDown, 1) + "\n" +
			"delay(0.02);\n" +
			post(cgRightMouseUp, 1)
	default:
		return fmt.Errorf("unknown mouse action: %s", input.Action)
	}

	script := "ObjC.import('CoreGraphics');\n" + body

	out, err := exec.Command("osascript", "-l", "JavaScript", "-e", script).CombinedOutput()
	if err != nil {
		return fmt.Errorf("mouse %s failed: %v — %s", input.Action, err, string(out))
	}
	return nil
}

// keyboardAction is the unified interface called by main.go
func keyboardAction(input KeyboardInput) error {
	if p := findCliclick(); p != "" {
		return cliclickKeyboardAction(p, input)
	}
	return applescriptKeyboardAction(input)
}

func cliclickKeyboardAction(bin string, input KeyboardInput) error {
	switch input.Action {
	case "type":
		return exec.Command(bin, fmt.Sprintf("t:%s", input.Text)).Run()
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

