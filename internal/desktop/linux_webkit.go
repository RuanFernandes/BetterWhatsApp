//go:build linux && cgo && !gtk3 && !server

package desktop

/*
#cgo linux pkg-config: webkitgtk-6.0
#cgo linux LDFLAGS: -Wl,--wrap=webkit_settings_new

#include <webkit/webkit.h>

// The Wails GTK4 backend creates a fresh WebKitSettings object for every
// window. Wrapping the constructor lets BetterWhatsApp configure the settings
// before Wails attaches them to a WebView and starts the first navigation.
extern WebKitSettings* __real_webkit_settings_new(void);

static const char betterwhatsapp_linux_webkit_user_agent[] =
    "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 "
    "(KHTML, like Gecko) Chrome/143.0.0.0 Safari/537.36";

WebKitSettings* __wrap_webkit_settings_new(void) {
    WebKitSettings *settings = __real_webkit_settings_new();
    if (settings == NULL) {
        return NULL;
    }

    webkit_settings_set_user_agent(settings, betterwhatsapp_linux_webkit_user_agent);
    webkit_settings_set_enable_site_specific_quirks(settings, FALSE);
    return settings;
}
*/
import "C"
