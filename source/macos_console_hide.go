//go:build darwin
// +build darwin

package main

import (
	"os"
	"os/exec"
	"syscall"
)

const detachedEnvFlag = "AURORA_DETACHED_NO_CONSOLE"

func isCharDevice(f *os.File) bool {
	if f == nil {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

// On macOS there is no windowsgui subsystem flag.
// To avoid running attached to an interactive console, we relaunch detached once.
func detachFromConsoleOnMacOS() {
	if DEBUG_MODE {
		return
	}
	if os.Getenv(detachedEnvFlag) == "1" {
		return
	}

	// Relaunch only when started from an interactive terminal.
	if !isCharDevice(os.Stdin) && !isCharDevice(os.Stdout) && !isCharDevice(os.Stderr) {
		return
	}

	devNull, err := os.OpenFile("/dev/null", os.O_RDWR, 0)
	if err != nil {
		return
	}
	defer devNull.Close()

	cmd := exec.Command(os.Args[0], os.Args[1:]...)
	cmd.Env = append(os.Environ(), detachedEnvFlag+"=1")
	cmd.Stdin = devNull
	cmd.Stdout = devNull
	cmd.Stderr = devNull
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err == nil {
		os.Exit(0)
	}
}

func init() {
	detachFromConsoleOnMacOS()
}

