//go:build linux

package desktop

const (
	// WebKitGTK reports web.whatsapp.com as an old Safari build unless the
	// embedder supplies a modern desktop identity. WhatsApp uses that identity
	// to select the desktop storage and login path.
	linuxWebKitUserAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.0.0 Safari/537.36"

	// WebKitGTK's site-specific quirk table replaces the embedder user agent
	// for web.whatsapp.com. Disable it so the identity above reaches both the
	// page and its network requests.
	linuxWebKitDisableSiteSpecificQuirks = true
)
