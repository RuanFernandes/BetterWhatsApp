//go:build !windows

package desktop

import "errors"

func nativeWindowVisible(uintptr) bool   { return false }
func nativeWindowMinimised(uintptr) bool { return false }
func focusNativeWindow(uintptr)          {}
func showNativeWindow(uintptr)           {}
func hideNativeWindow(uintptr)           {}

func toggleNativeMaximise(uintptr) error {
	return errors.New("native window controls are currently supported on Windows only")
}
