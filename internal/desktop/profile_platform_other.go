//go:build !windows

package desktop

import (
	"errors"
	"os/exec"
	"time"
)

func profileHostSupported() bool {
	return false
}

func startProfileProcess(string, string, uintptr, string) (*exec.Cmd, error) {
	return nil, errors.New("isolated profile windows are currently supported on Windows only")
}

func waitForProfileWindow(uint32, time.Duration) (uintptr, error) {
	return 0, errors.New("isolated profile windows are currently supported on Windows only")
}

func embedProfileWindow(uintptr, uintptr, int, int, int, int) error {
	return errors.New("isolated profile windows are currently supported on Windows only")
}

func resizeEmbeddedProfileWindow(uintptr, uintptr, int, int, int, int) error {
	return errors.New("isolated profile windows are currently supported on Windows only")
}

func setEmbeddedProfileVisibility(uintptr, bool) {}

func focusEmbeddedProfileWindow(uintptr) {}

func profileWindowProcessID(uintptr) uint32 {
	return 0
}

func platformWndProcInterceptor(func(uintptr, ProfileEvent) bool) func(uintptr, uint32, uintptr, uintptr) (uintptr, bool) {
	return nil
}

func sendProfileEvent(uintptr, uintptr, ProfileEvent) error {
	return errors.New("isolated profile windows are currently supported on Windows only")
}
