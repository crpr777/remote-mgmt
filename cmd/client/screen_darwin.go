//go:build darwin

package main

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
)

// captureScreen captures the screen on macOS using the screencapture command
func captureScreen(displayNum int) (*image.RGBA, image.Rectangle, error) {
	// Create temp file for screenshot
	tmpFile, err := os.CreateTemp("", "screenshot-*.png")
	if err != nil {
		return nil, image.Rectangle{}, fmt.Errorf("failed to create temp file: %v", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	// Use screencapture command
	// -x: no sound
	// -D: display number (1-indexed for screencapture)
	args := []string{"-x", "-D", fmt.Sprintf("%d", displayNum+1), tmpPath}
	cmd := exec.Command("screencapture", args...)
	if err := cmd.Run(); err != nil {
		return nil, image.Rectangle{}, fmt.Errorf("screencapture failed: %v", err)
	}

	// Read the image
	f, err := os.Open(tmpPath)
	if err != nil {
		return nil, image.Rectangle{}, fmt.Errorf("failed to open screenshot: %v", err)
	}
	defer f.Close()

	img, err := png.Decode(f)
	if err != nil {
		return nil, image.Rectangle{}, fmt.Errorf("failed to decode screenshot: %v", err)
	}

	// Convert to RGBA
	bounds := img.Bounds()
	rgba := image.NewRGBA(bounds)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			rgba.Set(x, y, img.At(x, y))
		}
	}

	return rgba, bounds, nil
}

// numDisplays returns the number of active displays
func numDisplays() int {
	// On macOS, we can check with system_profiler but it's slow
	// Default to 1, screencapture will handle multiple displays
	return 1
}

// getDisplayBounds returns the bounds for a display
func getDisplayBounds(displayNum int) image.Rectangle {
	// We'll get actual bounds from the captured image
	// Return a placeholder that will be updated
	return image.Rect(0, 0, 1920, 1080)
}
