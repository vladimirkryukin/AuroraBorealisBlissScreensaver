//go:build windows
// +build windows

package main

import "syscall"

// hideConsoleWindow hides attached console window on Windows startup.
// This keeps screensaver startup clean even if binary was built without
// `-ldflags "-H windowsgui"`.
func hideConsoleWindow() {
	if DEBUG_MODE {
		return
	}

	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	user32 := syscall.NewLazyDLL("user32.dll")
	procGetConsoleWindow := kernel32.NewProc("GetConsoleWindow")
	procShowWindow := user32.NewProc("ShowWindow")

	hwnd, _, _ := procGetConsoleWindow.Call()
	if hwnd == 0 {
		return
	}

	const SW_HIDE = 0
	procShowWindow.Call(hwnd, SW_HIDE)
}

func init() {
	hideConsoleWindow()
}

