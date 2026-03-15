package builder

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// BuildConfig holds the configuration for building a client
type BuildConfig struct {
	AuthKey        string
	TargetOS       string // "darwin" or "windows"
	TargetArch     string // "amd64" or "arm64"
	Version        string
	HostnamePrefix string
	OutputDir      string
}

// BuildResult contains the result of a build
type BuildResult struct {
	OutputPath string
	FileName   string
	Size       int64
	BuildTime  time.Duration
	Error      error
}

// BuildClient compiles the client binary with the auth key embedded
func BuildClient(cfg BuildConfig) BuildResult {
	startTime := time.Now()

	// Validate config
	if cfg.AuthKey == "" {
		return BuildResult{Error: fmt.Errorf("auth key is required")}
	}
	if cfg.TargetOS != "darwin" && cfg.TargetOS != "windows" {
		return BuildResult{Error: fmt.Errorf("target OS must be 'darwin' or 'windows'")}
	}
	if cfg.TargetArch == "" {
		cfg.TargetArch = "amd64"
	}
	if cfg.Version == "" {
		cfg.Version = "1.0.0"
	}
	if cfg.HostnamePrefix == "" {
		cfg.HostnamePrefix = "rmgmt"
	}

	// Determine output filename
	ext := ""
	if cfg.TargetOS == "windows" {
		ext = ".exe"
	}
	fileName := fmt.Sprintf("remote-mgmt-client-%s-%s%s", cfg.TargetOS, cfg.TargetArch, ext)
	outputPath := filepath.Join(cfg.OutputDir, fileName)

	// Build ldflags to inject variables at compile time
	buildTime := time.Now().UTC().Format(time.RFC3339)
	ldflags := fmt.Sprintf(
		"-s -w -X 'main.AuthKey=%s' -X 'main.Version=%s' -X 'main.BuildTime=%s' -X 'main.HostnamePrefix=%s'",
		cfg.AuthKey,
		cfg.Version,
		buildTime,
		cfg.HostnamePrefix,
	)

	// For Windows, add -H windowsgui to hide the console window
	if cfg.TargetOS == "windows" {
		ldflags += " -H windowsgui"
	}

	// Get the path to the client source
	// In production, this would be a fixed path or embedded
	clientPkg := "./cmd/client"

	// Build the binary
	cmd := exec.Command("go", "build",
		"-ldflags", ldflags,
		"-o", outputPath,
		clientPkg,
	)

	// Set environment for cross-compilation
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("GOOS=%s", cfg.TargetOS),
		fmt.Sprintf("GOARCH=%s", cfg.TargetArch),
		"CGO_ENABLED=0", // Disable CGO for easier cross-compilation
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return BuildResult{
			Error: fmt.Errorf("build failed: %v\nOutput: %s", err, string(output)),
		}
	}

	// Get file info
	info, err := os.Stat(outputPath)
	if err != nil {
		return BuildResult{Error: fmt.Errorf("failed to stat output: %v", err)}
	}

	return BuildResult{
		OutputPath: outputPath,
		FileName:   fileName,
		Size:       info.Size(),
		BuildTime:  time.Since(startTime),
	}
}

// SupportedTargets returns the list of supported OS/Arch combinations
func SupportedTargets() []struct {
	OS   string
	Arch string
	Name string
} {
	return []struct {
		OS   string
		Arch string
		Name string
	}{
		{"darwin", "amd64", "macOS (Intel)"},
		{"darwin", "arm64", "macOS (Apple Silicon)"},
		{"windows", "amd64", "Windows (64-bit)"},
		{"windows", "arm64", "Windows (ARM64)"},
	}
}
