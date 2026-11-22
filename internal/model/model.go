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
	"math"
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
	if len(u) >= 8 && strings.EqualFold(u[:8], "https://") {
		u = u[8:]
	} else if len(u) >= 7 && strings.EqualFold(u[:7], "http://") {
		u = u[7:]
	}
	if len(u) >= 4 && strings.EqualFold(u[:4], "www.") {
		u = u[4:]
	}
	if i := strings.IndexAny(u, "/?#"); i >= 0 {
		u = strings.ToLower(u[:i]) + u[i:]
	} else {
		u = strings.ToLower(u)
	}
	return strings.TrimSuffix(u, "/")
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

// Grid card sizes (Settings.CardSize).
const (
	CardCompact  = "compact"
	CardStandard = "standard"
	CardDetail   = "detail"
)

// CompactCardLayout holds the tunable geometry of compact grid cards.
type CompactCardLayout struct {
	NoteLines int // 0..4 (0 = title only, default 2)
	Width     int // 140..240 (default 176)
	Gap       int // 4..16 (default 8)
	Pad       int // 6..16 (default 10)
	Radius    int // 4..16 (default 9)
}

// Compact card presets.
var (
	CompactPresetTitle = CompactCardLayout{NoteLines: 0, Width: 156, Gap: 6, Pad: 10, Radius: 9}
	CompactPresetTwo   = CompactCardLayout{NoteLines: 2, Width: 176, Gap: 8, Pad: 10, Radius: 9}
	CompactPresetAiry  = CompactCardLayout{NoteLines: 3, Width: 212, Gap: 12, Pad: 13, Radius: 12}
)

// DefaultCompactCardLayout returns the default two-line note compact layout.
func DefaultCompactCardLayout() CompactCardLayout {
	return CompactPresetTwo
}

// Clamp bounds every compact layout field to its supported range.
func (c CompactCardLayout) Clamp() CompactCardLayout {
	c.NoteLines = max(0, min(4, c.NoteLines))
	c.Width = max(140, min(240, c.Width))
	c.Gap = max(4, min(16, c.Gap))
	c.Pad = max(6, min(16, c.Pad))
	c.Radius = max(4, min(16, c.Radius))
	return c
}

// NoteLineHeight returns the line height in DIPs for compact card notes.
func (CompactCardLayout) NoteLineHeight() float32 { return 16 }

// PadY returns the vertical padding in DIPs derived from Pad.
func (c CompactCardLayout) PadY() float32 {
	return max(float32(6), float32(math.Round(float64(c.Pad)*0.8)))
}

// NoteBoxHeight returns the reserved height in DIPs for compact card notes.
func (c CompactCardLayout) NoteBoxHeight() float32 {
	return float32(c.NoteLines) * c.NoteLineHeight()
}

// Height returns the total compact card height in DIPs.
func (c CompactCardLayout) Height() float32 {
	padY := c.PadY()
	if c.NoteLines > 0 {
		return padY*2 + 22 + 6 + c.NoteBoxHeight()
	}
	return max(float32(42), 22+padY*2)
}

// NoteOverflows estimates whether note exceeds the reserved compact lines
// at cardWidth, used to decide whether to show the trailing right-edge fade.
func (c CompactCardLayout) NoteOverflows(note string, cardWidth float32) bool {
	if note == "" || c.NoteLines <= 0 {
		return false
	}
	innerW := max(float32(48), cardWidth-float32(c.Pad)*2) * 0.94
	line := 1
	var x float32
	for _, r := range note {
		if r == '\n' {
			line++
			x = 0
		} else {
			w := float32(6.4)
			if r > 255 {
				w = 11.5
			}
			if x+w > innerW {
				line++
				x = w
			} else {
				x += w
			}
		}
		if line > c.NoteLines {
			return true
		}
	}
	return false
}

// NormalizeCardSize maps unknown or empty values to the standard size.
func NormalizeCardSize(v string) string {
	switch v {
	case CardCompact, CardDetail:
		return v
	}
	return CardStandard
}

// Settings are per device and never synced (settings.json in the app's
// user data dir).
type Settings struct {
	Appearance        string `json:"appearance"`                  // auto | light | dark
	DefaultView       string `json:"defaultView"`                 // list | grid
	CardSize          string `json:"cardSize,omitempty"`          // compact | standard | detail (grid view)
	CompactNoteLines  *int   `json:"compactNoteLines,omitempty"`  // 0..4 (nil = default 2)
	CompactCardWidth  int    `json:"compactCardWidth,omitempty"`  // 140..240 (0 = default 176)
	CompactCardGap    int    `json:"compactCardGap,omitempty"`    // 4..16 (0 = default 8)
	CompactCardPad    int    `json:"compactCardPad,omitempty"`    // 6..16 (0 = default 10)
	CompactCardRadius int    `json:"compactCardRadius,omitempty"` // 4..16 (0 = default 9)
	ShowNote          bool   `json:"showNote"`
	ShowTags          bool   `json:"showTags"`

	// OpenModes are the summon modes switched on, besides the main window.
	OpenModes          []string `json:"openModes"`
	EdgeSide           string   `json:"edgeSide"` // left | right
	GlobalSearchOff    bool     `json:"globalSearchOff,omitempty"`
	LaunchInBackground bool     `json:"launchInBackground,omitempty"`
	HideDockWhenClosed bool     `json:"hideDockWhenClosed,omitempty"`
	NotificationsOff   bool     `json:"notificationsOff,omitempty"`

	// Global shortcuts (Electron-style accelerators, e.g. "Alt+Space", "CmdOrCtrl+Shift+K").
	// ShortcutSearch defaults to "Alt+Space" when empty and GlobalSearchOff is false.
	ShortcutSearch   string `json:"shortcutSearch,omitempty"`
	ShortcutQuickAdd string `json:"shortcutQuickAdd,omitempty"`
	ShortcutNotch    string `json:"shortcutNotch,omitempty"`
	ShortcutEdge     string `json:"shortcutEdge,omitempty"`

	AutoBackup bool `json:"autoBackup"`

	UpdateSource string `json:"updateSource,omitempty"` // auto | mirror | github

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

// CompactCard returns the effective compact card layout with defaults and clamping.
func (s *Settings) CompactCard() CompactCardLayout {
	d := DefaultCompactCardLayout()
	out := d
	if s.CompactNoteLines != nil {
		out.NoteLines = *s.CompactNoteLines
	}
	if s.CompactCardWidth > 0 {
		out.Width = s.CompactCardWidth
	}
	if s.CompactCardGap > 0 {
		out.Gap = s.CompactCardGap
	}
	if s.CompactCardPad > 0 {
		out.Pad = s.CompactCardPad
	}
	if s.CompactCardRadius > 0 {
		out.Radius = s.CompactCardRadius
	}
	return out.Clamp()
}

// SetCompactCard stores a clamped compact card layout on Settings.
func (s *Settings) SetCompactCard(v CompactCardLayout) {
	v = v.Clamp()
	lines := v.NoteLines
	s.CompactNoteLines = &lines
	s.CompactCardWidth = v.Width
	s.CompactCardGap = v.Gap
	s.CompactCardPad = v.Pad
	s.CompactCardRadius = v.Radius
}

// DefaultSettings are the settings of a fresh install.
func DefaultSettings() Settings {
	return Settings{
		Appearance: "auto", DefaultView: "list",
		OpenModes: []string{ModeTray}, EdgeSide: "right",
		AutoBackup: true,
		Storage:    StorageLocal, AIEngine: AIOff, AIAutoPolish: true, LocalModel: "qwen3.5-0.8b",
		OpenAIURL:  "https://api.openai.com/v1",
		UseWebView: true,
	}
}

// DefaultSearchShortcut is the default global hotkey for Quick Search.
const DefaultSearchShortcut = "Alt+Space"

// GlobalSearchOn reports whether the system-wide quick search hotkey is enabled.
func (s *Settings) GlobalSearchOn() bool {
	return !s.GlobalSearchOff
}

// EffectiveSearchShortcut returns the active global shortcut for Quick Search,
// or "" when global search is disabled.
func (s *Settings) EffectiveSearchShortcut() string {
	if s.GlobalSearchOff {
		return ""
	}
	if v := strings.TrimSpace(s.ShortcutSearch); v != "" {
		return v
	}
	return DefaultSearchShortcut
}

// NotificationsOn reports whether desktop notifications are enabled.
func (s *Settings) NotificationsOn() bool {
	return !s.NotificationsOff
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

// HasSummonMode reports whether at least one summon mode (notch, tray, or edge) is enabled.
func (s *Settings) HasSummonMode() bool {
	return s.ModeOn(ModeNotch) || s.ModeOn(ModeTray) || s.ModeOn(ModeEdge)
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
