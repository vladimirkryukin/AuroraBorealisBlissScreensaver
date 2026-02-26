//go:build windows
// +build windows

// Windows-specific URL opener.
// Uses ShellExecuteW to delegate URL handling to the user's default browser.
package main

import (
	"log"
	"syscall"
	"unsafe"
)

var (
	shell32              = syscall.NewLazyDLL("shell32.dll")
	procShellExecuteW    = shell32.NewProc("ShellExecuteW")
)

// openURL opens URL in default browser on Windows using ShellExecute
func openURL(url string) error {
	// ShellExecuteW(hwnd, lpOperation, lpFile, lpParameters, lpDirectory, nShowCmd)
	// lpOperation = "open" (UTF-16)
	// lpFile = URL (UTF-16)
	// nShowCmd = SW_SHOWNORMAL = 1
	
	// "open" verb asks ShellExecute to use default action for the URL scheme.
	operationUTF16, _ := syscall.UTF16FromString("open")
	urlUTF16, _ := syscall.UTF16FromString(url)
	
	ret, _, err := procShellExecuteW.Call(
		0,                                    // hwnd (NULL)
		uintptr(unsafe.Pointer(&operationUTF16[0])), // lpOperation
		uintptr(unsafe.Pointer(&urlUTF16[0])),        // lpFile
		0,                                    // lpParameters (NULL)
		0,                                    // lpDirectory (NULL)
		1,                                    // nShowCmd (SW_SHOWNORMAL)
	)
	
	// ShellExecute returns a value > 32 on success
	if ret <= 32 {
		if err != nil {
			return err
		}
		log.Printf("ShellExecute failed with return code: %d", ret)
		return syscall.Errno(ret)
	}
	
	return nil
}
