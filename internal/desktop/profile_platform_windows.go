//go:build windows

package desktop

import (
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

const (
	wmCopyData = 0x004A

	wsChild       = 0x40000000
	wsPopup       = 0x80000000
	wsCaption     = 0x00C00000
	wsThickFrame  = 0x00040000
	wsSysMenu     = 0x00080000
	wsMinimiseBox = 0x00020000
	wsMaximiseBox = 0x00010000
	wsOverlapped  = 0x00CF0000
	wsExAppWindow = 0x00040000

	swpNoActivate   = 0x0010
	swpFrameChanged = 0x0020
	swHide          = 0
	swShow          = 5
	createNoWindow  = 0x08000000
)

type copyDataStruct struct {
	dwData uintptr
	cbData uint32
	lpData uintptr
}

var profileUser32 = syscall.NewLazyDLL("user32.dll")
var profileKernel32 = syscall.NewLazyDLL("kernel32.dll")

var (
	enumWindowsProc          = profileUser32.NewProc("EnumWindows")
	getWindowThreadProcessID = profileUser32.NewProc("GetWindowThreadProcessId")
	getClassNameProc         = profileUser32.NewProc("GetClassNameW")
	getWindowLongPtr         = profileUser32.NewProc("GetWindowLongPtrW")
	setWindowLongPtr         = profileUser32.NewProc("SetWindowLongPtrW")
	setParentProc            = profileUser32.NewProc("SetParent")
	setWindowPosProc         = profileUser32.NewProc("SetWindowPos")
	showWindowProc           = profileUser32.NewProc("ShowWindow")
	enumChildWindowsProc     = profileUser32.NewProc("EnumChildWindows")
	isWindowVisibleProc      = profileUser32.NewProc("IsWindowVisible")
	setForegroundWindowProc  = profileUser32.NewProc("SetForegroundWindow")
	sendMessageProc          = profileUser32.NewProc("SendMessageW")
	moveMemoryProc           = profileKernel32.NewProc("RtlMoveMemory")
)

var profileVisibilityMu sync.Mutex
var profileVisibilityGeneration = make(map[uintptr]uint64)

func profileHostSupported() bool {
	return true
}

func startProfileProcess(executable, profileID string, parentHandle uintptr, token string) (*exec.Cmd, error) {
	command := exec.Command(
		executable,
		"--profile-window",
		profileID,
		"--parent-hwnd",
		fmt.Sprintf("%d", parentHandle),
		"--ipc-token",
		token,
	)
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNoWindow}
	if err := command.Start(); err != nil {
		return nil, fmt.Errorf("start profile process %q: %w", profileID, err)
	}
	return command, nil
}

func waitForProfileWindow(pid uint32, timeout time.Duration) (uintptr, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var found uintptr
		callback := syscall.NewCallback(func(hwnd uintptr, _ uintptr) uintptr {
			if profileWindowProcessID(hwnd) == pid && profileWindowHasWebview(hwnd) {
				found = hwnd
				return 0
			}
			return 1
		})
		_, _, _ = enumWindowsProc.Call(callback, 0)
		if found != 0 {
			return found, nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return 0, fmt.Errorf("timed out waiting for process %d window", pid)
}

func profileWindowHasWebview(hwnd uintptr) bool {
	if hwnd == 0 {
		return false
	}
	found := false
	callback := syscall.NewCallback(func(child uintptr, _ uintptr) uintptr {
		className := make([]uint16, 256)
		length, _, _ := getClassNameProc.Call(
			child,
			uintptr(unsafe.Pointer(&className[0])),
			uintptr(len(className)),
		)
		if length > 0 && strings.HasPrefix(syscall.UTF16ToString(className[:length]), "Chrome_WidgetWin_") {
			found = true
			return 0
		}
		return 1
	})
	_, _, _ = enumChildWindowsProc.Call(hwnd, callback, 0)
	return found
}

func profileWindowHasVisibleWebview(hwnd uintptr) bool {
	if hwnd == 0 {
		return false
	}
	found := false
	callback := syscall.NewCallback(func(child uintptr, _ uintptr) uintptr {
		className := make([]uint16, 256)
		length, _, _ := getClassNameProc.Call(
			child,
			uintptr(unsafe.Pointer(&className[0])),
			uintptr(len(className)),
		)
		if length == 0 || !strings.HasPrefix(syscall.UTF16ToString(className[:length]), "Chrome_WidgetWin_") {
			return 1
		}
		visible, _, _ := isWindowVisibleProc.Call(child)
		if visible != 0 {
			found = true
			return 0
		}
		return 1
	})
	_, _, _ = enumChildWindowsProc.Call(hwnd, callback, 0)
	return found
}

func profileWindowProcessID(hwnd uintptr) uint32 {
	if hwnd == 0 {
		return 0
	}
	var pid uint32
	_, _, _ = getWindowThreadProcessID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	return pid
}

func embedProfileWindow(hwnd, parent uintptr, x, y, width, height int) error {
	if hwnd == 0 || parent == 0 {
		return errors.New("profile or shell native handle is unavailable")
	}
	style := getWindowLongPtrValue(hwnd, -16)
	style = (style &^ (wsPopup | wsCaption | wsThickFrame | wsSysMenu | wsMinimiseBox | wsMaximiseBox | wsOverlapped)) | wsChild
	setWindowLongPtrValue(hwnd, -16, style)

	exStyle := getWindowLongPtrValue(hwnd, -20)
	setWindowLongPtrValue(hwnd, -20, exStyle&^wsExAppWindow)

	// SetParent returns the previous parent, not a success flag. A zero
	// return is valid when the profile window was top-level before embedding.
	_, _, _ = setParentProc.Call(hwnd, parent)
	if err := resizeEmbeddedProfileWindow(hwnd, parent, x, y, width, height); err != nil {
		return err
	}
	setEmbeddedProfileVisibility(hwnd, false)
	return nil
}

func resizeEmbeddedProfileWindow(hwnd, parent uintptr, x, y, width, height int) error {
	if hwnd == 0 || parent == 0 {
		return errors.New("profile or shell native handle is unavailable")
	}
	if width < 1 || height < 1 {
		return errors.New("profile window size is invalid")
	}
	flags := uintptr(swpNoActivate | swpFrameChanged)
	result, _, callErr := setWindowPosProc.Call(
		hwnd,
		0,
		uintptr(x),
		uintptr(y),
		uintptr(width),
		uintptr(height),
		flags,
	)
	if result == 0 {
		if callErr != syscall.Errno(0) {
			return fmt.Errorf("SetWindowPos failed: %w", callErr)
		}
		return errors.New("SetWindowPos failed")
	}
	return nil
}

func getWindowLongPtrValue(hwnd uintptr, index int32) uintptr {
	result, _, _ := getWindowLongPtr.Call(hwnd, uintptr(index))
	return result
}

func setWindowLongPtrValue(hwnd uintptr, index int32, value uintptr) {
	_, _, _ = setWindowLongPtr.Call(hwnd, uintptr(index), value)
}

func setEmbeddedProfileVisibility(hwnd uintptr, visible bool) {
	if hwnd == 0 {
		return
	}
	profileVisibilityMu.Lock()
	profileVisibilityGeneration[hwnd]++
	generation := profileVisibilityGeneration[hwnd]
	profileVisibilityMu.Unlock()

	if !visible {
		_, _, _ = showWindowProc.Call(hwnd, swHide)
		showEmbeddedProfileChildren(hwnd, swHide)
		return
	}

	go revealEmbeddedProfileWindow(hwnd, generation)
}

func showEmbeddedProfileChildren(hwnd, command uintptr) {
	if hwnd == 0 {
		return
	}
	callback := syscall.NewCallback(func(child uintptr, _ uintptr) uintptr {
		_, _, _ = showWindowProc.Call(child, command)
		return 1
	})
	_, _, _ = enumChildWindowsProc.Call(hwnd, callback, 0)
}

func revealEmbeddedProfileWindow(hwnd uintptr, generation uint64) {
	for attempt := 0; attempt < 300; attempt++ {
		if !isProfileVisibilityGenerationCurrent(hwnd, generation) {
			return
		}
		_, _, _ = showWindowProc.Call(hwnd, swShow)
		showEmbeddedProfileChildren(hwnd, swShow)
		if profileWindowHasVisibleWebview(hwnd) {
			return
		}
		_, _, _ = showWindowProc.Call(hwnd, swHide)
		showEmbeddedProfileChildren(hwnd, swHide)
		time.Sleep(100 * time.Millisecond)
	}
}

func isProfileVisibilityGenerationCurrent(hwnd uintptr, generation uint64) bool {
	profileVisibilityMu.Lock()
	defer profileVisibilityMu.Unlock()
	return profileVisibilityGeneration[hwnd] == generation
}

func focusEmbeddedProfileWindow(hwnd uintptr) {
	if hwnd != 0 {
		_, _, _ = setForegroundWindowProc.Call(hwnd)
	}
}

func platformWndProcInterceptor(handler func(uintptr, ProfileEvent) bool) func(uintptr, uint32, uintptr, uintptr) (uintptr, bool) {
	return func(_ uintptr, message uint32, wParam, lParam uintptr) (uintptr, bool) {
		if message != wmCopyData || handler == nil {
			return 0, false
		}
		event, ok := readProfileEvent(wParam, lParam)
		if !ok {
			return 0, false
		}
		if handler(wParam, event) {
			return 0, true
		}
		return 0, false
	}
}

func readProfileEvent(sender, lParam uintptr) (ProfileEvent, bool) {
	if sender == 0 || lParam == 0 {
		return ProfileEvent{}, false
	}
	var data copyDataStruct
	_, _, _ = moveMemoryProc.Call(
		uintptr(unsafe.Pointer(&data)),
		lParam,
		uintptr(unsafe.Sizeof(data)),
	)
	if data.cbData == 0 || data.cbData > 64*1024 || data.lpData == 0 {
		return ProfileEvent{}, false
	}
	bytes := make([]byte, int(data.cbData))
	_, _, _ = moveMemoryProc.Call(
		uintptr(unsafe.Pointer(&bytes[0])),
		data.lpData,
		uintptr(len(bytes)),
	)
	var event ProfileEvent
	if err := json.Unmarshal(bytes, &event); err != nil {
		return ProfileEvent{}, false
	}
	if event.ProfileID == "" || event.Token == "" {
		return ProfileEvent{}, false
	}
	return event, true
}

func sendProfileEvent(parent, sender uintptr, event ProfileEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("encode profile IPC event: %w", err)
	}
	if len(data) > 64*1024 {
		return errors.New("profile IPC event is too large")
	}
	copyData := copyDataStruct{
		cbData: uint32(len(data)),
		lpData: uintptr(unsafe.Pointer(&data[0])),
	}
	result, _, callErr := sendMessageProc.Call(
		parent,
		wmCopyData,
		sender,
		uintptr(unsafe.Pointer(&copyData)),
	)
	if result == 0 && callErr != syscall.Errno(0) {
		return fmt.Errorf("SendMessageW failed: %w", callErr)
	}
	return nil
}
