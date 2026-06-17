//go:build !darwin

package main

// checkPlatformPermissions is a no-op on non-macOS platforms.
func checkPlatformPermissions() {}
