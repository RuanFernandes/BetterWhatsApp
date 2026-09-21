//go:build linux && cgo && !gtk3 && !server

package desktop

/*
#cgo linux pkg-config: webkitgtk-6.0
#include <webkit/webkit.h>
*/
import "C"

import "testing"

func TestLinuxWebKitSettingsWrapper(t *testing.T) {
	settings := C.webkit_settings_new()
	if settings == nil {
		t.Fatal("webkit_settings_new returned nil")
	}

	if got := C.GoString(C.webkit_settings_get_user_agent(settings)); got != linuxWebKitUserAgent {
		t.Fatalf("unexpected WebKit user agent: %q", got)
	}
	if C.webkit_settings_get_enable_site_specific_quirks(settings) != 0 {
		t.Fatal("site-specific WebKit quirks are still enabled")
	}
}
