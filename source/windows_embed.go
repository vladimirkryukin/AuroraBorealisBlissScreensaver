//go:build windows
// +build windows

// Windows-only helpers for `/p` preview mode.
//
// Windows screensaver panel passes a parent HWND and expects the preview to be
// embedded as a child window. GLFW does not expose HWND directly, so we bridge
// into Win32 via `user32.dll` calls.
package main

import (
	"log"
	"syscall"
	"time"
	"unsafe"

	"github.com/go-gl/glfw/v3.3/glfw"
)

var (
	user32               = syscall.NewLazyDLL("user32.dll")
	procSetParent        = user32.NewProc("SetParent")
	procFindWindow       = user32.NewProc("FindWindowW")
	procGetWindowLongPtr = user32.NewProc("GetWindowLongPtrW")
	procSetWindowLongPtr = user32.NewProc("SetWindowLongPtrW")
	procGetClientRect    = user32.NewProc("GetClientRect")
	procSetWindowPos     = user32.NewProc("SetWindowPos")
	procMoveWindow       = user32.NewProc("MoveWindow")
	procGetWindowRect    = user32.NewProc("GetWindowRect")
	procScreenToClient   = user32.NewProc("ScreenToClient")
	procClientToScreen   = user32.NewProc("ClientToScreen")
	procShowWindow       = user32.NewProc("ShowWindow")
	procGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
	procEnumWindows      = user32.NewProc("EnumWindows")
)

// getWindowHWND gets HWND of a GLFW window by finding the window with matching title
// Returns HWND or 0 if not found
func getWindowHWND(windowTitle string) uintptr {
	// Convert title to UTF-16 for FindWindowW
	titleUTF16, _ := syscall.UTF16FromString(windowTitle)
	var titlePtr *uint16
	if len(titleUTF16) > 0 {
		titlePtr = &titleUTF16[0]
	}

	// Try to find window with retries (window may not be registered immediately)
	var glfwHWND uintptr
	for i := 0; i < 20; i++ {
		glfwHWND, _, _ = procFindWindow.Call(0, uintptr(unsafe.Pointer(titlePtr)))
		if glfwHWND != 0 {
			return glfwHWND
		}
		// Small delay before retry
		time.Sleep(1 * time.Millisecond)
	}
	return 0
}

// hideWindow hides a GLFW window on Windows using SetWindowPos with SWP_HIDEWINDOW
// This is faster and more reliable than ShowWindow
func hideWindow(window *glfw.Window, windowTitle string) {
	glfwHWND := getWindowHWND(windowTitle)
	if glfwHWND != 0 {
		// Use SetWindowPos with SWP_HIDEWINDOW to hide immediately
		// SWP_HIDEWINDOW = 0x0080, SWP_NOMOVE = 0x0002, SWP_NOSIZE = 0x0001, SWP_NOZORDER = 0x0004
		const SWP_HIDEWINDOW = 0x0080
		const SWP_NOMOVE = 0x0002
		const SWP_NOSIZE = 0x0001
		const SWP_NOZORDER = 0x0004
		procSetWindowPos.Call(glfwHWND, 0, 0, 0, 0, 0, SWP_HIDEWINDOW|SWP_NOMOVE|SWP_NOSIZE|SWP_NOZORDER)
	}
}

// showWindow shows a GLFW window on Windows
func showWindow(window *glfw.Window, windowTitle string) {
	// Convert title to UTF-16 for FindWindowW
	titleUTF16, _ := syscall.UTF16FromString(windowTitle)
	var titlePtr *uint16
	if len(titleUTF16) > 0 {
		titlePtr = &titleUTF16[0]
	}

	// Find our window by title
	glfwHWND, _, _ := procFindWindow.Call(0, uintptr(unsafe.Pointer(titlePtr)))
	if glfwHWND != 0 {
		// SW_SHOW = 5
		procShowWindow.Call(glfwHWND, 5)
	}
}

// embedWindowIntoParent embeds GLFW window into parent HWND on Windows
// Returns the width and height of the parent window's client area
func embedWindowIntoParent(window *glfw.Window, parentHWND uintptr, windowTitle string) (int, int) {
	// Convert title to UTF-16 for FindWindowW
	titleUTF16, _ := syscall.UTF16FromString(windowTitle)
	var titlePtr *uint16
	if len(titleUTF16) > 0 {
		titlePtr = &titleUTF16[0]
	}

	// Find our window by title (workaround since GLFW doesn't expose HWND directly)
	glfwHWND, _, _ := procFindWindow.Call(0, uintptr(unsafe.Pointer(titlePtr)))

	if glfwHWND != 0 {
		// Get parent window client area size
		// Use GetClientRect to get the exact client area size (without borders)
		type RECT struct {
			Left, Top, Right, Bottom int32
		}
		var clientRect RECT
		ret, _, _ := procGetClientRect.Call(parentHWND, uintptr(unsafe.Pointer(&clientRect)))
		if ret == 0 {
			if DEBUG_MODE {
				log.Printf("Warning: GetClientRect failed for parent HWND: %d", parentHWND)
			}
			return 320, 240 // Fallback to default size
		}
		
		// Calculate exact client area size
		// GetClientRect returns coordinates relative to client area, so Left and Top are always 0
		// Right and Bottom give us the width and height
		width := clientRect.Right - clientRect.Left
		height := clientRect.Bottom - clientRect.Top
		
		if DEBUG_MODE {
			log.Printf("Parent window client area: Left=%d, Top=%d, Right=%d, Bottom=%d, Size=%dx%d",
				clientRect.Left, clientRect.Top, clientRect.Right, clientRect.Bottom, width, height)
		}

		// First connect GLFW window to the preview panel parent.
		// Order matters: setting WS_CHILD before SetParent can be flaky on some hosts.
		procSetParent.Call(glfwHWND, parentHWND)

		// Set window style to be a child window without border/caption
		// GWL_STYLE = -16 (must be converted to uintptr via int32)
		var gwlStyle int32 = -16
		const WS_CHILD = uintptr(0x40000000)
		const WS_VISIBLE = uintptr(0x10000000)
		const WS_POPUP = uintptr(0x80000000)
		const WS_BORDER = uintptr(0x00800000)
		const WS_CAPTION = uintptr(0x00C00000)
		const WS_DLGFRAME = uintptr(0x00400000)
		const WS_THICKFRAME = uintptr(0x00040000)
		const WS_SYSMENU = uintptr(0x00080000)
		const WS_MINIMIZEBOX = uintptr(0x00020000)
		const WS_MAXIMIZEBOX = uintptr(0x00010000)

		style, _, _ := procGetWindowLongPtr.Call(glfwHWND, uintptr(gwlStyle))
		// Remove all window decorations and popup style, add WS_CHILD
		style = style &^ (WS_POPUP | WS_BORDER | WS_CAPTION | WS_DLGFRAME | WS_THICKFRAME | WS_SYSMENU | WS_MINIMIZEBOX | WS_MAXIMIZEBOX)
		style = style | WS_CHILD | WS_VISIBLE
		procSetWindowLongPtr.Call(glfwHWND, uintptr(gwlStyle), style)

		// After setting WS_CHILD style, verify parent client area size again
		// Sometimes the size can change slightly after style change
		var finalRect RECT
		procGetClientRect.Call(parentHWND, uintptr(unsafe.Pointer(&finalRect)))
		finalWidth := finalRect.Right - finalRect.Left
		finalHeight := finalRect.Bottom - finalRect.Top
		
		// Use the final verified size
		if finalWidth != width || finalHeight != height {
			if DEBUG_MODE {
				log.Printf("Parent client area size changed after style update: %dx%d -> %dx%d", width, height, finalWidth, finalHeight)
			}
			width = finalWidth
			height = finalHeight
		}

		// Resize and position child to fill parent client area exactly.
		// Coordinates are relative to parent client space for WS_CHILD windows.
		// Use MoveWindow with exact client area coordinates (0, 0) and size
		// MoveWindow(hWnd, X, Y, nWidth, nHeight, bRepaint)
		// Note: For child windows, coordinates are relative to parent's client area
		retMove, _, _ := procMoveWindow.Call(glfwHWND, 0, 0, uintptr(width), uintptr(height), 1)
		if retMove == 0 && DEBUG_MODE {
			log.Printf("Warning: MoveWindow failed")
		}

		// Also call SetWindowPos to enforce final placement/sizing.
		// SWP_NOZORDER = 0x0004, SWP_NOACTIVATE = 0x0010
		const SWP_NOZORDER = 0x0004
		const SWP_NOACTIVATE = 0x0010
		// SetWindowPos(hWnd, hWndInsertAfter, X, Y, cx, cy, uFlags)
		// For child windows, X and Y are relative to parent's client area
		// Note: Don't use SWP_SHOWWINDOW here, we'll show the window explicitly after embedding
		retPos, _, _ := procSetWindowPos.Call(glfwHWND, 0, 0, 0, uintptr(width), uintptr(height), SWP_NOZORDER|SWP_NOACTIVATE)
		if retPos == 0 && DEBUG_MODE {
			log.Printf("Warning: SetWindowPos failed")
		}
		
		// Show window after embedding is complete
		// SW_SHOW = 5
		procShowWindow.Call(glfwHWND, 5)
		
		// Verify the window size after setting (for debugging)
		if DEBUG_MODE {
			var verifyRect RECT
			procGetClientRect.Call(glfwHWND, uintptr(unsafe.Pointer(&verifyRect)))
			verifyWidth := verifyRect.Right - verifyRect.Left
			verifyHeight := verifyRect.Bottom - verifyRect.Top
			log.Printf("GLFW window client area after embedding: %dx%d (expected: %dx%d)", verifyWidth, verifyHeight, width, height)
		}

		if DEBUG_MODE {
			log.Printf("Embedded preview window (HWND: %d) into parent window (HWND: %d), size: %dx%d", glfwHWND, parentHWND, width, height)
		}
		// Resize GLFW window to match parent size
		window.SetSize(int(width), int(height))
		return int(width), int(height)
	} else if DEBUG_MODE {
		log.Printf("Warning: Could not find GLFW window HWND for embedding")
	}
	return 320, 240 // Default size if embedding failed
}
