//go:build !darwin

package builder

import "fmt"

// BuildMacOSPkg is not supported on non-macOS build hosts.
func BuildMacOSPkg(binaryPath, outputDir, version string) (string, error) {
	return "", fmt.Errorf("macOS .pkg packaging requires building on macOS (pkgbuild is not available on this platform)")
}
