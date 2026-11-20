// Package model holds Cooool Mark's data: the synced library (groups,
// sub-groups, bookmarks, trash) and the per-device settings.
//
// A bookmark belongs to exactly one group and one sub-group (single
// membership). Every record carries UpdatedAt so two devices editing the
// same library through iCloud Drive / a sync folder merge record by record.
package model

import (
	"cmp"
	"crypto/rand"
	"encoding/hex"
	"slices"
	"strings"
	"time"
)

// SchemaVersion of bookmarks.json.
const SchemaVersion = 1

// NewID returns a random 12-hex-digit id.
func NewID() string {
	var b [6]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// Icon is how a bookmark's tile looks: an image fetched from the site
// (Image, a file name under icons/), else a colored square with a letter.
type Icon struct {
	Image  string `json:"image,omitempty"` // icons/<hash>.png, relative to the data dir
	Color  string `json:"color,omitempty"` // #rrggbb
	Letter string `json:"letter,omitempty"`
}

// Bookmark is one saved page. A URL containing {query} is a template:
// opening it asks for the query first.
type Bookmark struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	URL       string    `json:"url"`
	Note      string    `json:"note,omitempty"`
	Tags      []string  `json:"tags,omitempty"`
	Icon      Icon      `json:"icon"`
	GroupID   string    `json:"groupId"`
	SubID     string    `json:"subId"`
	Order     float64   `json:"order"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	// DeletedAt is kept only to ignore legacy trash entries when loading old files.
	DeletedAt *time.Time `json:"deletedAt,omitempty"`
	// Polished is set once the AI pass filled title/note/tags.
	Polished bool `json:"polished,omitempty"`
}

// IsTemplate reports whether the URL takes a query.
func (b *Bookmark) IsTemplate() bool { return strings.Contains(b.URL, "{query}") }

// InTrash reports whether the bookmark is a legacy trashed record.
func (b *Bookmark) InTrash() bool { return b.DeletedAt != nil }

// Sub is a sub-group of a group.
type Sub struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Order     float64   `json:"order"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Group is a top-level group, shown as a pill.
type Group struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Order     float64   `json:"order"`
	Subs      []Sub     `json:"subs"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Tombstone records a record deleted for good, so a merge with another
// device's older copy does not bring it back.
type Tombstone struct {
	ID        string    `json:"id"`
	DeletedAt time.Time `json:"deletedAt"`
}

// Library is the whole of bookmarks.json.
type Library struct {
	SchemaVersion int         `json:"schemaVersion"`
	Revision      int64       `json:"revision"`
	UpdatedAt     time.Time   `json:"updatedAt"`
	DeviceID      string      `json:"deviceId,omitempty"` // device that wrote last
	Groups        []Group     `json:"groups"`
	Bookmarks     []Bookmark  `json:"bookmarks"`
	Tombstones    []Tombstone `json:"tombstones,omitempty"`
}

// Group returns the group with the id, or nil.
func (l *Library) Group(id string) *Group {
	for i := range l.Groups {
		if l.Groups[i].ID == id {
			return &l.Groups[i]
		}
	}
	return nil
}

// Sub returns the sub-group, or nil.
func (l *Library) Sub(groupID, subID string) *Sub {
	g := l.Group(groupID)
	if g == nil {
		return nil
	}
	for i := range g.Subs {
		if g.Subs[i].ID == subID {
			return &g.Subs[i]
		}
	}
	return nil
}

// Bookmark returns the bookmark with the id (trash included), or nil.
func (l *Library) Bookmark(id string) *Bookmark {
	for i := range l.Bookmarks {
		if l.Bookmarks[i].ID == id {
			return &l.Bookmarks[i]
		}
	}
	return nil
}

// In returns the live bookmarks of a sub-group, in order.
func (l *Library) In(groupID, subID string) []*Bookmark {
	var out []*Bookmark
	for i := range l.Bookmarks {
		b := &l.Bookmarks[i]
		if !b.InTrash() && b.GroupID == groupID && b.SubID == subID {
			out = append(out, b)
		}
	}
	sortBookmarks(out)
	return out
}

// Live returns every active bookmark.
func (l *Library) Live() []*Bookmark {
	var out []*Bookmark
	for i := range l.Bookmarks {
		if !l.Bookmarks[i].InTrash() {
			out = append(out, &l.Bookmarks[i])
		}
	}
	return out
}

// FindURL returns the live bookmark saved with the URL (normalized), or nil.
func (l *Library) FindURL(url string) *Bookmark {
	n := NormalizeURL(url)
	for _, b := range l.Live() {
		if NormalizeURL(b.URL) == n {
			return b
		}
	}
	return nil
}

// NormalizeURL lowercases the scheme and host and drops a trailing slash
// and the scheme, for duplicate checks.
func NormalizeURL(u string) string {
	u = strings.TrimSpace(u)
	low := strings.ToLower(u)
	for _, p := range []string{"https://", "http://"} {
		if strings.HasPrefix(low, p) {
			u, low = u[len(p):], low[len(p):]
		}
	}
	low = strings.TrimPrefix(low, "www.")
	if i := strings.IndexAny(low, "/?#"); i >= 0 {
		low = low[:i] + u[len(u)-len(low)+i:]
	}
	return strings.TrimSuffix(low, "/")
}

func sortBookmarks(bs []*Bookmark) {
	slices.SortStableFunc(bs, func(a, b *Bookmark) int {
		return cmp.Compare(a.Order, b.Order)
	})
}

// Open modes: how the app is summoned besides its main window.
const (
	ModeWindow = "window" // the compact main window (always available)
	ModeNotch  = "notch"  // a black panel growing out of the screen's notch
	ModeTray   = "tray"   // a menu bar / tray flyout that can be torn off
	ModeEdge   = "edge"   // a handle on a screen edge sliding a panel out
)

// AI engines.
const (
	AIOff    = "off"
	AILocal  = "local"  // downloaded Qwen GGUF run by llama-server
	AIOpenAI = "openai" // any OpenAI compatible endpoint; key in the keychain
)

// Storage locations.
const (
	StorageLocal  = "local"  // the app's data dir on this device
	StorageICloud = "icloud" // iCloud Drive/Cooool Mark (macOS, iCloud for Windows)
	StorageFolder = "folder" // any folder, e.g. OneDrive, Dropbox, Syncthing
)

// Settings are per device and never synced (settings.json in the app's
// user data dir).
type Settings struct {
	Appearance  string `json:"appearance"`  // auto | light | dark
	DefaultView string `json:"defaultView"` // list | grid
	ShowNote    bool   `json:"showNote"`
	ShowTags    bool   `json:"showTags"`

	// OpenModes are the summon modes switched on, besides the main window.
	OpenModes []string `json:"openModes"`
	EdgeSide  string   `json:"edgeSide"` // left | right

	AutoBackup bool `json:"autoBackup"`

	Storage       string `json:"storage"` // local | icloud | folder
	StorageFolder string `json:"storageFolder,omitempty"`

	AIEngine     string `json:"aiEngine"`     // off | local | openai
	AIAutoPolish bool   `json:"aiAutoPolish"` // polish new bookmarks in the background
	LocalModel   string `json:"localModel"`   // qwen3.5-0.8b | qwen3.5-2b | qwen3.5-4b
	OpenAIURL    string `json:"openaiUrl"`
	OpenAIModel  string `json:"openaiModel"`
	// The OpenAI key lives in mygo.Secrets under "openai-key".

	// UseWebView renders pages that need JavaScript in a hidden webview
	// when plain HTTP finds no title.
	UseWebView bool `json:"useWebView"`

	// Last place, always restored on launch.
	LastGroup string `json:"lastGroup,omitempty"`
	LastSub   string `json:"lastSub,omitempty"`

	// Last group per summon mode, restored on launch.
	NotchGroup string `json:"notchGroup,omitempty"`
	TrayGroup  string `json:"trayGroup,omitempty"`
	EdgeGroup  string `json:"edgeGroup,omitempty"`
}

// DefaultSettings are the settings of a fresh install.
func DefaultSettings() Settings {
	return Settings{
		Appearance: "auto", DefaultView: "list",
		OpenModes: []string{ModeTray}, EdgeSide: "right",
		AutoBackup: true,
		Storage:    StorageLocal, AIEngine: AIOff, AIAutoPolish: true, LocalModel: "qwen3.5-0.8b",
		OpenAIURL: "https://api.openai.com/v1",
		UseWebView: true,
	}
}

// ModeOn reports whether an open mode is switched on.
func (s *Settings) ModeOn(mode string) bool {
	if mode == ModeWindow {
		return true
	}
	for _, m := range s.OpenModes {
		if m == mode {
			return true
		}
	}
	return false
}

// SetMode switches an open mode on or off.
func (s *Settings) SetMode(mode string, on bool) {
	out := make([]string, 0, len(s.OpenModes)+1)
	for _, m := range s.OpenModes {
		if m != mode {
			out = append(out, m)
		}
	}
	if on {
		out = append(out, mode)
	}
	s.OpenModes = out
}
