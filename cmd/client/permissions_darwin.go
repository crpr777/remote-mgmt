//go:build darwin

package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// checkPlatformPermissions verifies that Screen Recording and Accessibility
// permissions are granted. On first run, if permissions are missing, an
// AppleScript dialog is shown to guide the user to System Settings.
// On subsequent runs, only a log warning is emitted to avoid nagging.
func checkPlatformPermissions() {
	var missing []string

	if !hasScreenRecordingPermission() {
		missing = append(missing, "Screen Recording")
	}
	if !hasAccessibilityPermission() {
		missing = append(missing, "Accessibility")
	}

	if len(missing) == 0 {
		log.Println("macOS permissions OK: Screen Recording ✓, Accessibility ✓")
		return
	}

	log.Printf("⚠ Missing macOS permissions: %s", strings.Join(missing, ", "))

	// Only show the interactive dialog once (marker file prevents repeated prompts)
	markerPath := filepath.Join(getStateDir(), ".permissions-prompted")
	if _, err := os.Stat(markerPath); err == nil {
		log.Println("Grant permissions in: System Settings → Privacy & Security")
		return
	}

	showPermissionsPrompt(missing)
	os.WriteFile(markerPath, []byte(time.Now().Format(time.RFC3339)), 0644)
}

// hasScreenRecordingPermission attempts a test screen capture.
// If Screen Recording is denied, screencapture produces a tiny/empty file.
func hasScreenRecordingPermission() bool {
	tmpFile, err := os.CreateTemp("", "rmgmt-perm-check-*.png")
	if err != nil {
		return false
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	cmd := exec.Command("screencapture", "-x", tmpPath)
	if err := cmd.Run(); err != nil {
		return false
	}

	info, err := os.Stat(tmpPath)
	if err != nil {
		return false
	}
	// A valid screenshot is typically many KB; denied captures are empty or tiny
	return info.Size() > 1024
}

// hasAccessibilityPermission tests whether System Events can be controlled.
func hasAccessibilityPermission() bool {
	cmd := exec.Command("osascript", "-e",
		`tell application "System Events" to get name of first process`)
	return cmd.Run() == nil
}

// showPermissionsPrompt displays an AppleScript dialog listing the missing
// permissions and offers to open the relevant System Settings panes.
func showPermissionsPrompt(missing []string) {
	permList := ""
	for _, p := range missing {
		permList += fmt.Sprintf("  \\u2022  %s\\n", p)
	}

	msg := fmt.Sprintf(
		"Remote Management requires macOS permissions:\\n\\n%s\\n"+
			"Open System Settings \\u2192 Privacy & Security and look for 'Remote Management' in each list.\\n\\n"+
			"Click 'Open Settings' to go there now.",
		permList,
	)

	script := fmt.Sprintf(
		`set dialogResult to display dialog "%s" buttons {"Later", "Open Settings"} `+
			`default button "Open Settings" with title "Remote Management — Permissions" with icon caution`+"\n"+
			`if button returned of dialogResult is "Open Settings" then`+"\n"+
			`    do shell script "open 'x-apple.systempreferences:com.apple.preference.security?Privacy_ScreenCapture'"`+"\n"+
			`    delay 1.5`+"\n"+
			`    do shell script "open 'x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility'"`+"\n"+
			`end if`,
		msg,
	)

	exec.Command("osascript", "-e", script).Run()
}
