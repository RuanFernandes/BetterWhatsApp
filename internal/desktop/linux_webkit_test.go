//go:build linux

package desktop

import "testing"

func TestLinuxWebKitIdentityMatchesWhatsAppDesktop(t *testing.T) {
	if linuxWebKitUserAgent != "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.0.0 Safari/537.36" {
		t.Fatalf("unexpected Linux WebKit user agent: %q", linuxWebKitUserAgent)
	}
	if !linuxWebKitDisableSiteSpecificQuirks {
		t.Fatal("site-specific WebKit quirks must be disabled for WhatsApp Web")
	}
}
