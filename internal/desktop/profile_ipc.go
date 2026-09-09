package desktop

import "errors"

type ProfileEvent struct {
	ProfileID string `json:"profileId"`
	Token     string `json:"token"`
	Type      string `json:"type"`
	Count     int    `json:"count,omitempty"`
}

func SendProfileEvent(parentHandle, senderHandle uintptr, event ProfileEvent) error {
	if parentHandle == 0 {
		return errors.New("parent shell handle is unavailable")
	}
	if senderHandle == 0 {
		return errors.New("profile sender handle is unavailable")
	}
	if event.ProfileID == "" || event.Token == "" {
		return errors.New("profile IPC identity is incomplete")
	}
	switch event.Type {
	case "unread", "new-message":
	default:
		return errors.New("unsupported profile IPC event")
	}
	return sendProfileEvent(parentHandle, senderHandle, event)
}
