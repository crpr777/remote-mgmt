//go:build windows

package main

import (
	"image"

	"github.com/kbinani/screenshot"
)

// captureScreen captures the screen on Windows using kbinani/screenshot
func captureScreen(displayNum int) (*image.RGBA, image.Rectangle, error) {
	n := screenshot.NumActiveDisplays()
	if displayNum >= n {
		displayNum = 0
	}

	bounds := screenshot.GetDisplayBounds(displayNum)
	img, err := screenshot.CaptureRect(bounds)
	if err != nil {
		return nil, image.Rectangle{}, err
	}

	return img, bounds, nil
}

// numDisplays returns the number of active displays
func numDisplays() int {
	return screenshot.NumActiveDisplays()
}

// getDisplayBounds returns the bounds for a display
func getDisplayBounds(displayNum int) image.Rectangle {
	return screenshot.GetDisplayBounds(displayNum)
}
