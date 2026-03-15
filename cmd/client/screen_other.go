//go:build !windows && !darwin

package main

import (
	"fmt"
	"image"
)

// captureScreen is a stub for unsupported platforms
func captureScreen(displayNum int) (*image.RGBA, image.Rectangle, error) {
	return nil, image.Rectangle{}, fmt.Errorf("screen capture not supported on this platform")
}

// numDisplays returns the number of active displays
func numDisplays() int {
	return 0
}

// getDisplayBounds returns the bounds for a display
func getDisplayBounds(displayNum int) image.Rectangle {
	return image.Rect(0, 0, 0, 0)
}
