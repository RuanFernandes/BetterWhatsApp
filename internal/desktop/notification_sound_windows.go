//go:build windows

package desktop

import "syscall"

const messageBeepInformation = 0x00000040

var user32 = syscall.NewLazyDLL("user32.dll")
var messageBeep = user32.NewProc("MessageBeep")

func playNotificationSound() {
	_, _, _ = messageBeep.Call(messageBeepInformation)
}
