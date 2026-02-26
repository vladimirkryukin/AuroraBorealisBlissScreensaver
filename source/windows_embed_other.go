//go:build !windows
// +build !windows

// Non-Windows stubs for preview embedding APIs.
// These functions keep build targets portable while preview embedding remains
// implemented only through Win32 calls in `windows_embed.go`.
package main

import (
	"github.com/go-gl/glfw/v3.3/glfw"
)

// hideWindow is a no-op on non-Windows platforms
func hideWindow(window *glfw.Window, windowTitle string) {
	// No-op on non-Windows
}

// showWindow is a no-op on non-Windows platforms
func showWindow(window *glfw.Window, windowTitle string) {
	// No-op on non-Windows
}

// embedWindowIntoParent is a stub for non-Windows platforms
func embedWindowIntoParent(window *glfw.Window, parentHWND uintptr, windowTitle string) (int, int) {
	// Not implemented on non-Windows platforms
	return 320, 240 // Default size
}
