package widget

import (
	"net/url"
	"strings"

	"github.com/egoist/mygo/ui"
)

// TypedSearchKey listens for a bare letter (a-z / Shift+A-Z) or digit (0-9)
// keypress when no text input is focused, returning the typed character so
// views can immediately enter search mode with that character prefilled.
func TypedSearchKey(c *ui.Context) (string, bool) {
	for key := ui.Key0; key <= ui.Key9; key++ {
		if c.Shortcut(0, key) && !c.ShortcutRepeated() {
			return string(rune('0' + (key - ui.Key0))), true
		}
	}
	for key := ui.KeyA; key <= ui.KeyZ; key++ {
		if c.Shortcut(0, key) && !c.ShortcutRepeated() {
			return string(rune('a' + (key - ui.KeyA))), true
		}
		if c.Shortcut(ui.Shift, key) && !c.ShortcutRepeated() {
			return string(rune('A' + (key - ui.KeyA))), true
		}
	}
	return "", false
}

// LooksLikeURL reports whether q looks like a bare domain or http(s) URL.
func LooksLikeURL(q string) bool {
	q = strings.TrimSpace(q)
	if q == "" || strings.ContainsAny(q, " \t\n\r") {
		return false
	}
	low := strings.ToLower(q)
	if strings.HasPrefix(low, "http://") || strings.HasPrefix(low, "https://") {
		u, err := url.Parse(q)
		return err == nil && u.Host != ""
	}
	dot := strings.IndexByte(q, '.')
	if dot <= 0 || dot >= len(q)-1 {
		return false
	}
	host := q
	if slash := strings.IndexAny(host, "/?#"); slash >= 0 {
		host = host[:slash]
	}
	parts := strings.Split(host, ".")
	if len(parts) < 2 {
		return false
	}
	tld := parts[len(parts)-1]
	return len(tld) >= 2 && !strings.ContainsAny(tld, "0123456789")
}

// EnsureURLScheme prepends https:// when q has no http(s) scheme.
func EnsureURLScheme(q string) string {
	low := strings.ToLower(q)
	if strings.HasPrefix(low, "http://") || strings.HasPrefix(low, "https://") {
		return q
	}
	return "https://" + q
}

// ShortDomain formats a bookmark URL cleanly for secondary display.
func ShortDomain(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		s := strings.TrimPrefix(strings.TrimPrefix(raw, "https://"), "http://")
		s = strings.TrimPrefix(s, "www.")
		return s
	}
	host := strings.TrimPrefix(u.Host, "www.")
	if u.Path != "" && u.Path != "/" {
		return host + u.Path
	}
	return host
}
