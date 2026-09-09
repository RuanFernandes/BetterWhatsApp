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

	wsChild        = 0x40000000
	wsClipChildren = 0x02000000
	wsClipSiblings = 0x04000000
	wsVisible      = 0x10000000
	wsPopup        = 0x80000000
	wsCaption      = 0x00C00000
	wsThickFrame   = 0x00040000
	wsSysMenu      = 0x00080000
	wsMinimiseBox  = 0x00020000
	wsMaximiseBox  = 0x00010000
	wsOverlapped   = 0x00CF0000
	wsExAppWindow  = 0x00040000

	swpNoSize        = 0x0001
	swpNoMove        = 0x0002
	swpNoActivate    = 0x0010
	swpShowWindow    = 0x0040
	swpNoOwnerZOrder = 0x0200
	swpFrameChanged  = 0x0020
	swHide           = 0
	swShow           = 5
	createNoWindow   = 0x08000000
)

type copyDataStruct struct {
	dwData uintptr
	cbData uint32
	lpData uintptr
}

var profileUser32 = syscall.NewLazyDLL("user32.dll")
var profileKernel32 = syscall.NewLazyDLL("kernel32.dll")

var (
	createWindowExProc       = profileUser32.NewProc("CreateWindowExW")
	destroyWindowProc        = profileUser32.NewProc("DestroyWindow")
	enumWindowsProc          = profileUser32.NewProc("EnumWindows")
	getWindowThreadProcessID = profileUser32.NewProc("GetWindowThreadProcessId")
	getClassNameProc         = profileUser32.NewProc("GetClassNameW")
	getClientRectProc        = profileUser32.NewProc("GetClientRect")
	getDpiForWindowProc      = profileUser32.NewProc("GetDpiForWindow")
	getWindowLongPtr         = profileUser32.NewProc("GetWindowLongPtrW")
	setWindowLongPtr         = profileUser32.NewProc("SetWindowLongPtrW")
	setParentProc            = profileUser32.NewProc("SetParent")
	setWindowPosProc         = profileUser32.NewProc("SetWindowPos")
	showWindowProc           = profileUser32.NewProc("ShowWindow")
	enumChildWindowsProc     = profileUser32.NewProc("EnumChildWindows")
	isWindowVisibleProc      = profileUser32.NewProc("IsWindowVisible")
	setForegroundWindowProc  = profileUser32.NewProc("SetForegroundWindow")
	sendMessageProc          = profileUser32.NewProc("SendMessageW")
	getModuleHandleProc      = profileKernel32.NewProc("GetModuleHandleW")
	moveMemoryProc           = profileKernel32.NewProc("RtlMoveMemory")
)

type profileRect struct {
	left   int32
	top    int32
	right  int32
	bottom int32
}

var profileVisibilityMu sync.Mutex
var profileVisibilityGeneration = make(map[uintptr]uint64)

func profileHostSupported() bool {
	return true
}

func createProfileHost(parent uintptr) (uintptr, error) {
	if parent == 0 {
		return 0, errors.New("shell native window is unavailable")
	}
	className, err := syscall.UTF16PtrFromString("STATIC")
	if err != nil {
		return 0, fmt.Errorf("encode profile host class: %w", err)
	}
	module, _, _ := getModuleHandleProc.Call(0)
	hwnd, _, callErr := createWindowExProc.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		0,
		uintptr(wsChild|wsClipChildren|wsClipSiblings|wsVisible),
		0,
		0,
		0,
		0,
		parent,
		0,
		module,
		0,
	)
	if hwnd == 0 {
		if callErr != syscall.Errno(0) {
			return 0, fmt.Errorf("CreateWindowExW failed: %w", callErr)
		}
		return 0, errors.New("CreateWindowExW failed")
	}
	return hwnd, nil
}

func destroyProfileHost(hwnd uintptr) {
	if hwnd != 0 {
		_, _, _ = destroyWindowProc.Call(hwnd)
	}
}

func resizeProfileHost(hwnd, parent uintptr, headerDip int) error {
	if hwnd == 0 || parent == 0 {
		return errors.New("profile host or shell native window is unavailable")
	}
	width, height, ok := profileWindowClientSize(parent)
	if !ok || width < 1 || height < 1 {
		return errors.New("shell client size is invalid")
	}
	header := dipToPhysical(headerDip, profileWindowDPI(parent))
	if header < 0 {
		header = 0
	}
	if header >= height {
		header = height - 1
	}
	return setProfileWindowPosition(hwnd, 0, header, width, height-header, swpNoActivate|swpNoOwnerZOrder|swpShowWindow|swpFrameChanged)
}

func profileHostClientSize(hwnd uintptr) (int, int, bool) {
	return profileWindowClientSize(hwnd)
}

func profileWindowClientSize(hwnd uintptr) (int, int, bool) {
	if hwnd == 0 {
		return 0, 0, false
	}
	var rect profileRect
	result, _, _ := getClientRectProc.Call(hwnd, uintptr(unsafe.Pointer(&rect)))
	if result == 0 {
		return 0, 0, false
	}
	width := int(rect.right - rect.left)
	height := int(rect.bottom - rect.top)
	return width, height, width > 0 && height > 0
}

func profileWindowDPI(hwnd uintptr) uint32 {
	if hwnd == 0 {
		return 96
	}
	dpi, _, _ := getDpiForWindowProc.Call(hwnd)
	if dpi == 0 {
		return 96
	}
	return uint32(dpi)
}

func dipToPhysical(value int, dpi uint32) int {
	if value <= 0 {
		return value
	}
	return int((int64(value)*int64(dpi) + 48) / 96)
}

func setProfileWindowPosition(hwnd uintptr, x, y, width, height, flags int) error {
	if hwnd == 0 || width < 1 || height < 1 {
		return errors.New("profile window size is invalid")
	}
	result, _, callErr := setWindowPosProc.Call(
		hwnd,
		0,
		uintptr(x),
		uintptr(y),
		uintptr(width),
		uintptr(height),
		uintptr(flags),
	)
	if result == 0 {
		if callErr != syscall.Errno(0) {
			return fmt.Errorf("SetWindowPos failed: %w", callErr)
		}
		return errors.New("SetWindowPos failed")
	}
	return nil
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
	return setProfileWindowPosition(hwnd, x, y, width, height, swpNoActivate|swpNoOwnerZOrder|swpShowWindow|swpFrameChanged)
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

func activateEmbeddedProfileWindow(parent, hwnd uintptr) {
	if parent != 0 {
		// Keep the shell active so its custom frame remains painted and owns the
		// drag/resize hit testing. Raise the foreign-process child without
		// activating it.
		_, _, _ = setForegroundWindowProc.Call(parent)
	}
	if hwnd != 0 {
		_ = setProfileWindowPosition(hwnd, 0, 0, 1, 1, swpNoMove|swpNoSize|swpNoActivate|swpShowWindow)
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
