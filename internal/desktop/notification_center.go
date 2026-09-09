package desktop

import (
	"fmt"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const notificationSoundCooldown = 2 * time.Second

type NotificationCenter struct {
	mu sync.Mutex

	tray            *application.SystemTray
	unreadMenuItem  *application.MenuItem
	normalIcon      []byte
	unreadIcon      []byte
	unreadCount     int
	unreadByProfile map[string]int
	lastSoundAt     time.Time
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
		normalIcon:      append([]byte(nil), normalIcon...),
		unreadIcon:      append([]byte(nil), unreadIcon...),
		unreadByProfile: make(map[string]int),
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

	n.apply(snapshot)
}

func (n *NotificationCenter) SetUnreadCount(count int) {
	if n == nil {
		return
	}

	n.mu.Lock()
	n.unreadByProfile = make(map[string]int)
	n.unreadCount = normalizeUnreadCount(count)
	snapshot := n.snapshotLocked()
	n.mu.Unlock()

	n.apply(snapshot)
}

func (n *NotificationCenter) SetProfileUnreadCount(profileID string, count int) {
	if n == nil || profileID == "" {
		return
	}

	n.mu.Lock()
	n.unreadByProfile[profileID] = normalizeUnreadCount(count)
	total := 0
	for _, profileCount := range n.unreadByProfile {
		total += profileCount
		if total >= 999999 {
			total = 999999
			break
		}
	}
	n.unreadCount = total
	snapshot := n.snapshotLocked()
	n.mu.Unlock()

	n.apply(snapshot)
}

func (n *NotificationCenter) ClearProfileUnreadCount(profileID string) {
	if n == nil || profileID == "" {
		return
	}

	n.mu.Lock()
	delete(n.unreadByProfile, profileID)
	total := 0
	for _, profileCount := range n.unreadByProfile {
		total += profileCount
		if total >= 999999 {
			total = 999999
			break
		}
	}
	n.unreadCount = total
	snapshot := n.snapshotLocked()
	n.mu.Unlock()

	n.apply(snapshot)
}

func (n *NotificationCenter) HandleNewMessage(window application.Window) {
	if n == nil || window == nil || (window.IsVisible() && !window.IsMinimised()) {
		return
	}

	now := time.Now()
	n.mu.Lock()
	if !n.lastSoundAt.IsZero() && now.Sub(n.lastSoundAt) < notificationSoundCooldown {
		n.mu.Unlock()
		return
	}
	n.lastSoundAt = now
	n.mu.Unlock()

	go playNotificationSound()
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

func (n *NotificationCenter) apply(snapshot notificationSnapshot) {
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
