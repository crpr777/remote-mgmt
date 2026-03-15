# Remote Management Tool

A lightweight remote management framework built in Go. Uses [Tailscale](https://tailscale.com/) (embedded WireGuard mesh) as the transport layer — no port forwarding, no exposed services on the public internet, all traffic E2E encrypted.

## Architecture

```
Browser → Admin Server (:8000) → [Tailscale / WireGuard mesh] → Client Agent (:8080)
```

Two components:

- **Admin Server** — Web UI for building client binaries, discovering endpoints on the tailnet, and remote-controlling them (screen view, input, command exec, file browser).
- **Client Agent** — Zero-config single binary deployed to endpoints. Tailscale auth key is baked in at compile time. Listens only on its Tailscale interface, unreachable from the public internet.

```
┌─────────────────────────────────────────────────────────┐
│                    Admin Web UI (:8000)                  │
│  • Build clients with embedded Tailscale auth keys      │
│  • Cross-compile for Windows / macOS                    │
│  • Proxy all traffic through server's Tailscale conn    │
└─────────────────────────┬───────────────────────────────┘
                          │ proxies via tailnet
                          ▼
┌─────────────────────────────────────────────────────────┐
│              Client Agent (single binary)                │
│  • Embedded Tailscale via tsnet                         │
│  • Auth key baked in at compile time                    │
│  • Zero configuration for end users                     │
│  • Joins your tailnet automatically                     │
│  • Screen capture, input injection, command exec        │
└─────────────────────────┬───────────────────────────────┘
                          │ connected via
                          ▼
┌─────────────────────────────────────────────────────────┐
│                 Tailscale Network                        │
│  • WireGuard encrypted P2P mesh                         │
│  • ACL-based access control                             │
│  • DERP relay fallback when direct P2P isn't possible   │
└─────────────────────────────────────────────────────────┘
```

## Design Decisions

- **Tailscale as the network layer**: Handles NAT traversal, encryption, authentication, and ACL-based access control in a single dependency. Falls back to DERP relays when direct connections fail.
- **tsnet (embedded Tailscale)**: Neither component requires a separate Tailscale installation — each binary carries its own Tailscale node.
- **Compile-time credential injection**: Auth keys are embedded via Go `-ldflags -X` at build time, producing self-contained binaries with zero end-user configuration.
- **SSE-based screen streaming**: Frames are JPEG-encoded, base64'd, and pushed over Server-Sent Events. A pixel-sampling hash skips unchanged frames. Simpler than WebRTC, works through the HTTP proxy.
- **Go build tags for platform code**: Screen capture, input control, and process hiding are in `_darwin.go`, `_windows.go`, and `_other.go` files, selected at compile time.
- **Server-side proxy**: The `/proxy/{clientIP}/...` route uses `httputil.ReverseProxy` with the Tailscale transport, so the browser only ever talks to the admin server.

## Supported Platforms

| Target | Screen Capture | Input Control |
|--------|---------------|---------------|
| macOS Intel (darwin/amd64) | `screencapture` CLI | cliclick / AppleScript |
| macOS Apple Silicon (darwin/arm64) | `screencapture` CLI | cliclick / AppleScript |
| Windows 64-bit (windows/amd64) | GDI via `kbinani/screenshot` | Win32 `SendInput` API |
| Windows ARM (windows/arm64) | GDI via `kbinani/screenshot` | Win32 `SendInput` API |

## Project Structure

```
remote-mgmt/
├── cmd/client/
│   ├── main.go               # Client agent: API server, screen capture, input, exec, files
│   ├── screen_darwin.go       # macOS screen capture (screencapture CLI → PNG → RGBA)
│   ├── screen_windows.go      # Windows screen capture (kbinani/screenshot, GDI)
│   ├── screen_other.go        # Stub for unsupported platforms
│   ├── input_darwin.go        # macOS input injection (cliclick / AppleScript fallback)
│   ├── input_windows.go       # Windows input injection (SendInput, SetCursorPos via syscall)
│   ├── input_other.go         # Stub
│   ├── platform_windows.go    # Windows-specific: hide console window (CREATE_NO_WINDOW)
│   └── platform_other.go      # No-op on non-Windows
├── internal/builder/
│   └── builder.go             # Cross-compilation: go build with ldflags injection
├── web/
│   ├── server.go              # Admin server: build API, client discovery, proxy, SSE relay
│   └── templates/
│       ├── index.html         # Build page — compile client binaries
│       ├── clients.html       # Endpoint discovery & management
│       └── viewer.html        # Remote desktop viewer with control panel
├── go.mod
├── Makefile
└── README.md
```

## Prerequisites

- **Go 1.21+** — https://go.dev/dl/
- **Tailscale account** — https://tailscale.com/

## Quick Start

### 1. Install & Run the Admin Server

```bash
go mod tidy

# Option A: Join your tailnet (recommended)
export TS_AUTHKEY="tskey-auth-xxx"
go run web/server.go --hostname rmgmt-admin

# Option B: Local only (your machine needs Tailscale installed separately)
go run web/server.go --local-only
```

Access at `http://localhost:8000` (or `http://rmgmt-admin` via Tailscale).

**Server flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--auth-key` | `$TS_AUTHKEY` | Tailscale auth key for the server |
| `--hostname` | `rmgmt-admin` | Tailscale hostname |
| `--state-dir` | `~/.rmgmt-admin` | Tailscale state directory |
| `--local-only` | `false` | Run without Tailscale |
| `--bind` | `:8000` | Local bind address |

### 2. Build a Client Binary

**Via the Web UI:**
1. Generate a Tailscale auth key at https://login.tailscale.com/admin/settings/keys (Reusable, Pre-authorized, tagged `tag:managed-client`)
2. Paste the key in the Build page, select a platform, click Compile
3. Download the binary

**Via Make:**
```bash
make client-darwin-arm64 AUTH_KEY=tskey-auth-xxx
make client-windows-amd64 AUTH_KEY=tskey-auth-xxx
```

### 3. Deploy

Users just run the binary — no configuration needed:

```bash
# macOS
chmod +x remote-mgmt-client-darwin-arm64
./remote-mgmt-client-darwin-arm64

# Windows
remote-mgmt-client-windows-amd64.exe
```

The client automatically joins the tailnet and appears in the admin UI.

## Admin UI Features

- **Build** (`/`) — Cross-compile client binaries with embedded auth keys
- **Clients** (`/clients`) — Auto-discover endpoints on your tailnet, probe status, manual connect
- **Viewer** (`/viewer`) — Remote desktop with:
  - Live screen streaming (SSE + JPEG, adjustable quality/FPS)
  - Mouse & keyboard input forwarding
  - Command execution
  - File browser

## Client Agent API

The client exposes these endpoints on port 8080 (Tailscale network only):

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/` | GET | Client info (hostname, OS, version, uptime) |
| `/ping` | GET | Health check |
| `/system` | GET | System details |
| `/screen/capture` | GET | Single screenshot (JPEG, configurable quality) |
| `/screen/stream` | GET | SSE stream of screen frames |
| `/input/mouse` | POST | Mouse actions (move, click, doubleclick, rightclick, scroll) |
| `/input/keyboard` | POST | Keyboard actions (type, keydown, keyup, hotkey) |
| `/exec` | POST | Run a command with timeout |
| `/files/list` | GET | List directory contents |
| `/files/read` | GET | Read a file (≤10MB) |
| `/files/write` | POST | Write a file |

## Tailscale ACL Configuration

Recommended ACLs for controlling access:

```json
{
  "tagOwners": {
    "tag:managed-client": ["autogroup:admin"],
    "tag:admin": ["autogroup:admin"]
  },
  "acls": [
    {
      "action": "accept",
      "src": ["tag:admin"],
      "dst": ["tag:managed-client:*"]
    },
    {
      "action": "accept",
      "src": ["tag:managed-client"],
      "dst": ["tag:admin:*"]
    }
  ]
}
```

## Security Considerations

- **Auth keys expire** — Tailscale auth keys have a max lifetime of 90 days. Plan for rotation.
- **Binary = credential** — The auth key is embedded in compiled binaries. Treat them as sensitive.
- **Use tags** — Always use tagged auth keys so ACLs apply correctly.
- **Key scoping** — Use different keys for different deployment batches to limit blast radius.
- **macOS permissions** — Screen Recording and Accessibility permissions required on clients.
- **Windows UIPI** — Input injection won't work on elevated/admin applications.

## Roadmap

- [x] Remote desktop streaming (SSE-based)
- [x] Remote input control (mouse + keyboard)
- [x] File browser
- [x] Command execution
- [ ] System monitoring / metrics dashboard
- [ ] Installer packages (.pkg, .msi)
- [ ] Client auto-update mechanism
- [ ] WebRTC for lower-latency streaming

## License

MIT
