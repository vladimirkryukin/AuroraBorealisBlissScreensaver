//go:build !windows
// +build !windows

// Non-Windows URL opener implementation.
// Used for local development/testing when `/c` dialog triggers website links.
package main

import (
	"log"
	"os/exec"
	"runtime"
)

// openURL opens URL in default browser on non-Windows platforms
func openURL(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	default:
		log.Printf("Unsupported OS for opening URL: %s", runtime.GOOS)
		return nil
	}
	return cmd.Run()
}
