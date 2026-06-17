#!/bin/bash
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
