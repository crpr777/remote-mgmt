================================================================================
                     REMOTE MANAGEMENT TOOL - README
================================================================================

OVERVIEW
--------
A lightweight remote management solution using Tailscale for secure 
peer-to-peer encrypted communication. Consists of an Admin Server (for 
building and managing clients) and Client binaries (deployed to endpoints).


================================================================================
                            ADMIN SERVER SETUP
================================================================================

PREREQUISITES
-------------
  - Go 1.21 or later (https://go.dev/dl/)
  - A Tailscale account (https://tailscale.com/)


INSTALLATION
------------
  1. Navigate to the project directory:
       cd ~/dev-projects/remote-mgmt

  2. Install dependencies:
       go mod tidy

  3. Build the admin server:
       go build -o admin-server ./web/


RUNNING THE SERVER
------------------

Option A: With Tailscale (Recommended)
  Connects the admin server to your tailnet for auto-discovery and remote access.

    export TS_AUTHKEY="tskey-auth-xxx-your-admin-key"
    ./admin-server --hostname rmgmt-admin

  Or with flags:
    ./admin-server --auth-key "tskey-auth-xxx" --hostname rmgmt-admin

  Access at:
    - http://localhost:8000 (local)
    - http://rmgmt-admin (via Tailscale)

Option B: Local Only Mode
  Run without joining Tailscale (requires Tailscale installed separately):

    ./admin-server --local-only

  Access at: http://localhost:8000


SERVER FLAGS
------------
  --auth-key     Tailscale auth key (default: $TS_AUTHKEY env var)
  --hostname     Tailscale hostname (default: rmgmt-admin)
  --state-dir    Tailscale state directory (default: ~/.rmgmt-admin)
  --local-only   Run without Tailscale (default: false)


BUILDING CLIENT BINARIES
------------------------
  1. Generate a Tailscale auth key:
     - Go to https://login.tailscale.com/admin/settings/keys
     - Click "Generate auth key"
     - Settings: Reusable=Yes, Ephemeral=No, Pre-authorized=Yes
     - Add a tag like "tag:managed-client"

  2. Open the Admin UI in your browser (http://localhost:8000)

  3. Paste the auth key and select target platform

  4. Click "Build Client Binary" and download

  Alternatively, build via command line:
    make client-darwin-arm64 AUTH_KEY=tskey-auth-xxx
    make client-windows-amd64 AUTH_KEY=tskey-auth-xxx


ADMIN UI FEATURES
-----------------
  /            Build page - compile client binaries with embedded auth keys
  /clients     Manage endpoints - view status, auto-discovery
  /viewer      Remote desktop - screen streaming, input control, file browser


================================================================================
                            CLIENT INSTALLATION
================================================================================

WINDOWS
-------
  1. Double-click: remote-mgmt-client-windows-amd64.exe

  2. If Windows Firewall prompts, click "Allow"
     - If dismissed/denied, client still works via DERP relays (slower)

  3. The client runs in the background - no further action needed


macOS
-----
  1. Open Terminal

  2. Make executable:
       chmod +x remote-mgmt-client-darwin-arm64

  3. Run:
       ./remote-mgmt-client-darwin-arm64

  4. Grant permissions when prompted (Screen Recording, Accessibility)


CLIENT PERMISSIONS
------------------

Windows:
  - No special permissions or admin rights required
  - No screen capture permissions needed at user level
  - Input to elevated (Run as Admin) apps may be blocked by UIPI

macOS:
  - Screen Recording permission required (System Settings > Privacy & Security)
  - Accessibility permission required for remote input control


FIREWALL NOTES
--------------
  - No admin rights needed to run
  - Windows Firewall will likely prompt on first run
  - If firewall prompt is dismissed/denied, Tailscale still works via 
    DERP relays (just with higher latency)


WHAT THE CLIENT DOES
--------------------
  - Automatically joins the management network
  - Appears in the administrator's console
  - Listens for control commands (only accessible via secure Tailscale network)
  - All traffic is encrypted end-to-end using WireGuard


STOPPING THE CLIENT
-------------------
  Windows: Close the window or use Task Manager
  macOS:   Press Ctrl+C or close the terminal


================================================================================
                             TROUBLESHOOTING
================================================================================

"Connection seems slow"
  - Ensure firewall allows direct connections
  - Check network isn't heavily restricted

"Client won't start"
  - Verify execute permissions (macOS: chmod +x)
  - Check antivirus isn't blocking

"Remote control not working"
  - macOS: Grant Screen Recording and Accessibility permissions
  - Windows: Won't work on elevated/admin applications (UIPI)

"Server can't find clients"
  - Ensure server is running with Tailscale (not --local-only)
  - Verify clients are online in Tailscale admin console
  - Check ACL rules allow communication

For additional help, see README.md or contact your administrator.

================================================================================
