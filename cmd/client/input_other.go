//go:build !windows && !darwin

package main

import "fmt"

// mouseAction is the unified interface called by main.go
func mouseAction(input MouseInput) error {
	return fmt.Errorf("input control not supported on this platform")
}

// keyboardAction is the unified interface called by main.go
func keyboardAction(input KeyboardInput) error {
	return fmt.Errorf("input control not supported on this platform")
}
