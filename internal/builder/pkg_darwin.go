//go:build darwin

package builder

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
)

const (
	pkgIdentifier  = "com.remotemgmt.client"
	appName        = "RemoteMgmt.app"
	appInstallDir  = "/Applications"
	appExecName    = "remote-mgmt" // CFBundleExecutable inside the .app
	launchAgentDir = "/Library/LaunchAgents"
)

// launchAgentPlist is the template for the LaunchAgent plist file.
// Installed to /Library/LaunchAgents/ so it loads for all users on login.
var launchAgentPlistTmpl = template.Must(template.New("plist").Parse(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>{{ .Identifier }}</string>
    <key>ProgramArguments</key>
    <array>
        <string>{{ .BinPath }}</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <dict>
        <key>SuccessfulExit</key>
        <false/>
    </dict>
    <key>StandardOutPath</key>
    <string>/tmp/remote-mgmt-client.log</string>
    <key>StandardErrorPath</key>
    <string>/tmp/remote-mgmt-client.log</string>
    <key>ProcessType</key>
    <string>Interactive</string>
</dict>
</plist>
`))

// infoPlistTmpl is the Info.plist for the .app bundle.
// LSUIElement=true makes it a background agent (no Dock icon).
// The NS*UsageDescription keys provide context in TCC permission prompts.
var infoPlistTmpl = template.Must(template.New("info").Parse(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleExecutable</key>
    <string>{{ .ExecName }}</string>
    <key>CFBundleIdentifier</key>
    <string>{{ .Identifier }}</string>
    <key>CFBundleName</key>
    <string>Remote Management</string>
    <key>CFBundleDisplayName</key>
    <string>Remote Management</string>
    <key>CFBundlePackageType</key>
    <string>APPL</string>
    <key>CFBundleVersion</key>
    <string>{{ .Version }}</string>
    <key>CFBundleShortVersionString</key>
    <string>{{ .Version }}</string>
    <key>LSUIElement</key>
    <true/>
    <key>LSBackgroundOnly</key>
    <true/>
    <key>NSScreenCaptureUsageDescription</key>
    <string>Remote Management needs Screen Recording access for remote desktop.</string>
    <key>NSAppleEventsUsageDescription</key>
    <string>Remote Management needs to control System Events for remote input.</string>
</dict>
</plist>
`))

// postinstallScript replicates what Sentinel does (ad-hoc sign + unquarantine),
// adds the app to Gatekeeper's allowlist, loads the LaunchAgent, and prompts
// the user to grant Screen Recording and Accessibility permissions.
const postinstallScript = `#!/bin/bash
set -e

APP_PATH="{{APP_PATH}}"
BIN="{{BIN_PATH}}"
PLIST="{{PLIST_PATH}}"
IDENTIFIER="{{IDENTIFIER}}"

# --- Sentinel-equivalent: sign + unquarantine ---

# 1. Remove quarantine extended attribute from the entire .app bundle
xattr -cr "$APP_PATH" 2>/dev/null || true
xattr -cr "$PLIST" 2>/dev/null || true

# 2. Ad-hoc codesign the .app (same as Sentinel's "sign" feature)
codesign -s - --force --deep "$APP_PATH" 2>/dev/null || true

# 3. Add to Gatekeeper allowlist so macOS won't block it
spctl --add --label "$IDENTIFIER" "$APP_PATH" 2>/dev/null || true

# 4. Reset TCC permissions for our bundle so macOS re-prompts after re-signing
#    (ad-hoc signing changes the code hash, invalidating old permission grants)
tccutil reset ScreenCapture "$IDENTIFIER" 2>/dev/null || true
tccutil reset Accessibility "$IDENTIFIER" 2>/dev/null || true

# 5. Clear the first-run permissions marker so the dialog shows again
CONSOLE_HOME=$( eval echo "~$(/usr/bin/stat -f '%Su' /dev/console 2>/dev/null)" 2>/dev/null || echo "" )
if [ -n "$CONSOLE_HOME" ]; then
    rm -f "$CONSOLE_HOME/Library/Application Support/RemoteMgmt/.permissions-prompted" 2>/dev/null || true
fi

# Ensure binary is executable
chmod 755 "$BIN"

# Determine the console user
CONSOLE_USER=$(/usr/bin/stat -f "%Su" /dev/console 2>/dev/null || echo "")

if [ -n "$CONSOLE_USER" ] && [ "$CONSOLE_USER" != "root" ]; then
    CONSOLE_UID=$(id -u "$CONSOLE_USER" 2>/dev/null || echo "")

    # Load LaunchAgent
    if [ -n "$CONSOLE_UID" ]; then
        launchctl bootout "gui/$CONSOLE_UID/$IDENTIFIER" 2>/dev/null || true
        launchctl bootstrap "gui/$CONSOLE_UID" "$PLIST" 2>/dev/null || true
    fi

    # Prompt user to grant required macOS permissions
    PERM_SCRIPT=$(mktemp /tmp/rmgmt-perms-XXXXXX)
    cat > "$PERM_SCRIPT" << 'ENDAPPLESCRIPT'
set dialogResult to display dialog "Remote Management has been installed successfully." & return & return & "This app requires macOS permissions to function:" & return & return & "  \u2022  Screen Recording  \u2014  for remote desktop" & return & "  \u2022  Accessibility  \u2014  for keyboard & mouse control" & return & return & "Open System Settings \u2192 Privacy & Security and look for 'Remote Management' in each list." & return & return & "Click 'Open Settings' to go there now." buttons {"Later", "Open Settings"} default button "Open Settings" with title "Remote Management \u2014 Setup" with icon caution
if button returned of dialogResult is "Open Settings" then
    do shell script "open 'x-apple.systempreferences:com.apple.preference.security?Privacy_ScreenCapture'"
    delay 1.5
    do shell script "open 'x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility'"
end if
ENDAPPLESCRIPT
    sudo -u "$CONSOLE_USER" osascript "$PERM_SCRIPT" 2>/dev/null || true
    rm -f "$PERM_SCRIPT"
fi

exit 0
`

// BuildMacOSPkg wraps a compiled darwin binary into a .pkg installer using pkgbuild.
// The .pkg installs a proper .app bundle to /Applications so the agent appears
// in macOS Privacy settings (Screen Recording, Accessibility) by name.
// It also installs a LaunchAgent plist and runs a postinstall that strips
// quarantine and loads the agent.
func BuildMacOSPkg(binaryPath, outputDir, version string) (pkgPath string, err error) {
	// Verify pkgbuild is available
	if _, err := exec.LookPath("pkgbuild"); err != nil {
		return "", fmt.Errorf("pkgbuild not found — install Xcode Command Line Tools: xcode-select --install")
	}

	binName := filepath.Base(binaryPath)
	pkgName := strings.TrimSuffix(binName, filepath.Ext(binName)) + ".pkg"
	pkgPath = filepath.Join(outputDir, pkgName)

	// Paths as they will exist on the target machine
	installedAppPath := filepath.Join(appInstallDir, appName)
	installedBinPath := filepath.Join(installedAppPath, "Contents", "MacOS", appExecName)
	plistName := pkgIdentifier + ".plist"
	installedPlistPath := filepath.Join(launchAgentDir, plistName)

	// Create staging directories
	stageDir, err := os.MkdirTemp("", "rmgmt-pkg-stage-")
	if err != nil {
		return "", fmt.Errorf("failed to create staging dir: %w", err)
	}
	defer os.RemoveAll(stageDir)

	payloadDir := filepath.Join(stageDir, "payload")
	scriptsDir := filepath.Join(stageDir, "scripts")

	// Mirror the install hierarchy: .app bundle in /Applications, LaunchAgent in /Library
	appMacOSDir := filepath.Join(payloadDir, "Applications", appName, "Contents", "MacOS")
	appContentsDir := filepath.Join(payloadDir, "Applications", appName, "Contents")
	payloadAgentDir := filepath.Join(payloadDir, "Library", "LaunchAgents")

	for _, d := range []string{appMacOSDir, payloadAgentDir, scriptsDir} {
		if err := os.MkdirAll(d, 0755); err != nil {
			return "", fmt.Errorf("mkdir %s: %w", d, err)
		}
	}

	// Copy binary into .app bundle
	binData, err := os.ReadFile(binaryPath)
	if err != nil {
		return "", fmt.Errorf("read binary: %w", err)
	}
	if err := os.WriteFile(filepath.Join(appMacOSDir, appExecName), binData, 0755); err != nil {
		return "", fmt.Errorf("write binary to app bundle: %w", err)
	}

	// Generate Info.plist for the .app bundle
	infoPlistFile, err := os.Create(filepath.Join(appContentsDir, "Info.plist"))
	if err != nil {
		return "", fmt.Errorf("create Info.plist: %w", err)
	}
	err = infoPlistTmpl.Execute(infoPlistFile, struct {
		ExecName   string
		Identifier string
		Version    string
	}{
		ExecName:   appExecName,
		Identifier: pkgIdentifier,
		Version:    version,
	})
	infoPlistFile.Close()
	if err != nil {
		return "", fmt.Errorf("write Info.plist: %w", err)
	}

	// Generate LaunchAgent plist into payload
	plistFile, err := os.Create(filepath.Join(payloadAgentDir, plistName))
	if err != nil {
		return "", fmt.Errorf("create LaunchAgent plist: %w", err)
	}
	err = launchAgentPlistTmpl.Execute(plistFile, struct {
		Identifier string
		BinPath    string
	}{
		Identifier: pkgIdentifier,
		BinPath:    installedBinPath,
	})
	plistFile.Close()
	if err != nil {
		return "", fmt.Errorf("write LaunchAgent plist: %w", err)
	}

	// Generate postinstall script
	script := postinstallScript
	script = strings.ReplaceAll(script, "{{APP_PATH}}", installedAppPath)
	script = strings.ReplaceAll(script, "{{BIN_PATH}}", installedBinPath)
	script = strings.ReplaceAll(script, "{{PLIST_PATH}}", installedPlistPath)
	script = strings.ReplaceAll(script, "{{IDENTIFIER}}", pkgIdentifier)
	postinstallPath := filepath.Join(scriptsDir, "postinstall")
	if err := os.WriteFile(postinstallPath, []byte(script), 0755); err != nil {
		return "", fmt.Errorf("write postinstall: %w", err)
	}

	// Run pkgbuild
	cmd := exec.Command("pkgbuild",
		"--root", payloadDir,
		"--identifier", pkgIdentifier,
		"--version", version,
		"--install-location", "/",
		"--scripts", scriptsDir,
		pkgPath,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("pkgbuild failed: %v\nOutput: %s", err, string(output))
	}

	return pkgPath, nil
}
