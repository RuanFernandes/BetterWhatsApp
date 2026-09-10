//go:build windows

package desktop

import (
	"errors"
	"fmt"
	"syscall"
)

const (
	swHide     = 0
	swShow     = 5
	swMaximise = 3
	swRestore  = 9
)

var windowUser32 = syscall.NewLazyDLL("user32.dll")

var (
	showWindowProc          = windowUser32.NewProc("ShowWindow")
	isZoomedProc            = windowUser32.NewProc("IsZoomed")
	isWindowVisibleProc     = windowUser32.NewProc("IsWindowVisible")
	setForegroundWindowProc = windowUser32.NewProc("SetForegroundWindow")
)

func nativeWindowVisible(hwnd uintptr) bool {
	if hwnd == 0 {
		return false
	}
	visible, _, _ := isWindowVisibleProc.Call(hwnd)
	return visible != 0
}

func focusNativeWindow(hwnd uintptr) {
	if hwnd == 0 {
		return
	}
	_, _, _ = setForegroundWindowProc.Call(hwnd)
}

func showNativeWindow(hwnd uintptr) {
	if hwnd == 0 {
		return
	}
	_, _, _ = showWindowProc.Call(hwnd, swShow)
	focusNativeWindow(hwnd)
}

func hideNativeWindow(hwnd uintptr) {
	if hwnd == 0 {
		return
	}
	_, _, _ = showWindowProc.Call(hwnd, swHide)
}

func toggleNativeMaximise(hwnd uintptr) error {
	if hwnd == 0 {
		return errors.New("BetterWhatsApp native window is not ready")
	}
	maximised, _, _ := isZoomedProc.Call(hwnd)
	command := uintptr(swMaximise)
	if maximised != 0 {
		command = swRestore
	}
	_, _, callErr := showWindowProc.Call(hwnd, command)
	if callErr != nil && callErr != syscall.Errno(0) {
		return fmt.Errorf("ShowWindow failed: %w", callErr)
	}
	return nil
}
