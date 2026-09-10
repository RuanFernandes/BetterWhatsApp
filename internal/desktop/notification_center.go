package desktop

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type NotificationCenter struct {
	mu sync.Mutex

	tray            *application.SystemTray
	unreadMenuItem  *application.MenuItem
	normalIcon      []byte
	unreadIcon      []byte
	unreadCount     int
	asyncReady      atomic.Bool
	applyPending    bool
	pendingSnapshot *notificationSnapshot
}

type notificationSnapshot struct {
	tray           *application.SystemTray
	unreadMenuItem *application.MenuItem
	icon           []byte
	tooltip        string
	menuLabel      string
}

func NewNotificationCenter(normalIcon, unreadIcon []byte) *NotificationCenter {
	return &NotificationCenter{
		normalIcon: append([]byte(nil), normalIcon...),
		unreadIcon: append([]byte(nil), unreadIcon...),
	}
}

func (n *NotificationCenter) AttachTray(tray *application.SystemTray, unreadMenuItem *application.MenuItem) {
	if n == nil {
		return
	}

	n.mu.Lock()
	n.tray = tray
	n.unreadMenuItem = unreadMenuItem
	snapshot := n.snapshotLocked()
	n.mu.Unlock()

	n.enqueueApply(snapshot)
}

// Start switches tray updates to the Wails main-thread queue after the native
// application loop has started. AttachTray applies the initial state before
// Run, so startup does not need to touch the native tray again.
func (n *NotificationCenter) Start() {
	if n == nil {
		return
	}

	n.asyncReady.Store(true)
}

func (n *NotificationCenter) SetUnreadCount(count int) {
	if n == nil {
		return
	}

	n.mu.Lock()
	n.unreadCount = normalizeUnreadCount(count)
	snapshot := n.snapshotLocked()
	n.mu.Unlock()

	n.enqueueApply(snapshot)
}

func (n *NotificationCenter) snapshotLocked() notificationSnapshot {
	count := n.unreadCount
	icon := n.normalIcon
	if count > 0 && len(n.unreadIcon) > 0 {
		icon = n.unreadIcon
	}
	label := unreadLabel(count)
	return notificationSnapshot{
		tray:           n.tray,
		unreadMenuItem: n.unreadMenuItem,
		icon:           icon,
		tooltip:        "BetterWhatsApp — " + label,
		menuLabel:      label,
	}
}

func (n *NotificationCenter) enqueueApply(snapshot notificationSnapshot) {
	if n == nil {
		return
	}

	if !n.asyncReady.Load() {
		n.applyNow(snapshot)
		return
	}

	n.mu.Lock()
	if n.applyPending {
		copy := snapshot
		n.pendingSnapshot = &copy
		n.mu.Unlock()
		return
	}
	n.applyPending = true
	n.mu.Unlock()

	application.InvokeAsync(func() {
		n.runPendingApplies(snapshot)
	})
}

func (n *NotificationCenter) runPendingApplies(snapshot notificationSnapshot) {
	for {
		n.applyNow(snapshot)

		n.mu.Lock()
		if n.pendingSnapshot == nil {
			n.applyPending = false
			n.mu.Unlock()
			return
		}
		snapshot = *n.pendingSnapshot
		n.pendingSnapshot = nil
		n.mu.Unlock()
	}
}

func (n *NotificationCenter) applyNow(snapshot notificationSnapshot) {
	if snapshot.tray != nil {
		if len(snapshot.icon) > 0 {
			snapshot.tray.SetIcon(snapshot.icon)
		}
		snapshot.tray.SetTooltip(snapshot.tooltip)
	}
	if snapshot.unreadMenuItem != nil {
		snapshot.unreadMenuItem.SetLabel(snapshot.menuLabel)
	}
}

func normalizeUnreadCount(count int) int {
	if count < 0 {
		return 0
	}
	if count > 999999 {
		return 999999
	}
	return count
}

func unreadLabel(count int) string {
	switch count {
	case 0:
		return "Nenhuma mensagem não lida"
	case 1:
		return "1 mensagem não lida"
	default:
		return fmt.Sprintf("%d mensagens não lidas", count)
	}
}
