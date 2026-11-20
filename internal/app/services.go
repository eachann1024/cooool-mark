package app

import (
	"context"

	"cooool-mark/internal/model"
)

// The services are interfaces so the views compile against contracts and
// each service lives in its own package (internal/fetch, internal/search,
// internal/ai, internal/importer, internal/linkcheck). Every method may be
// called from any goroutine unless it says otherwise; slow ones take a ctx.

// PageMeta is what Fetch learns about a page.
type PageMeta struct {
	URL         string // final URL after redirects
	Title       string // cleaned: site suffixes like " - GitHub" trimmed when redundant
	Description string
	SiteName    string
	Icon        []byte // best icon found (apple-touch-icon, SVG, manifest, favicon), PNG/SVG/ICO bytes
	IconExt     string // ".png", ".svg", ".ico", ...
}

// Fetcher reads pages.
type Fetcher interface {
	// Fetch reads a page over HTTP honoring the system proxy, and, when
	// that finds no title and UseWebView is on, renders it in a hidden
	// webview to read the title after JavaScript ran.
	Fetch(ctx context.Context, url string) (PageMeta, error)
}

// Hit is a search result: the bookmark and the rune ranges of its title
// that matched, for highlighting.
type Hit struct {
	B      *model.Bookmark
	Ranges [][2]int // [start, end) rune offsets into B.Title
	Path   string   // "group / sub"
}

// Searcher finds bookmarks by title, URL, note and tags, in Chinese by
// pinyin initials and full pinyin too (ssp → 少数派). Called on the main
// thread; must stay under a few ms for 10k bookmarks.
type Searcher interface {
	Search(lib *model.Library, query string, groupID string) []Hit
}

// GroupChoice is a place a bookmark can go, for AI suggestions.
type GroupChoice struct {
	GroupID, SubID string
	Path           string // "开发 / 工具"
}

// PolishInput is what the AI sees of a new bookmark.
type PolishInput struct {
	URL, Title, Description string
	Choices                 []GroupChoice
}

// Polish is the AI's suggestion; empty fields mean no suggestion.
type Polish struct {
	Title   string
	Note    string
	Tags    []string
	GroupID string
	SubID   string
}

// AIStatus is shown in Settings and decides whether the editor offers a
// suggestion.
type AIStatus struct {
	Engine      string // the engine in use: model.AILocal, model.AIOpenAI or model.AIOff
	EngineName  string // "Apple 系统模型", "Qwen3.5-2B（本地）", ...
	Ready       bool
	Downloading bool
	Paused      bool
	Progress    float64 // 0..1 while downloading the local model
	Downloaded  int64   // bytes
	Total       int64
	Err         string
}

// AI polishes bookmarks in the background. It never blocks saving.
type AI interface {
	Status() AIStatus
	LocalModelStatus(model string) AIStatus
	// Configure applies the AI settings (engine, URLs, model names).
	Configure(s model.Settings)
	Polish(ctx context.Context, in PolishInput) (Polish, error)
	// Download fetches the local GGUF model (resumable), Pause stops it
	// keeping the part, Remove deletes the model.
	Download(model string) error
	Pause()
	Remove(model string) error
	// OnChange is called (any goroutine) when Status changes.
	OnChange(fn func())
	// OnDownloadComplete is called (any goroutine) when a local model finishes downloading.
	OnDownloadComplete(fn func(modelName string))
	// Close stops active downloads and any running local llama-server child process.
	Close()
}

// ImportFolder is a folder of an imported file, shown as a tree with
// check boxes before importing.
type ImportFolder struct {
	Name     string
	Items    []model.Bookmark // only Title, URL, CreatedAt set
	Children []*ImportFolder
	Checked  bool
	Dups     int // items already saved
	Count    int // items here and below
}

// ImportPlan is a parsed file.
type ImportPlan struct {
	File    string
	Format  string          // "html" (Netscape bookmarks, every browser) or "json" (our backup)
	Folders []*ImportFolder // top level
	Backup  *model.Library  // set for our JSON backups
	Total   int
	Dups    int
}

// Import modes.
const (
	ImportMerge   = "merge"   // top folders into same-named groups, else new groups
	ImportRestore = "restore" // JSON backup replaces the library (snapshot first)
)

// ImportOptions choose how a plan applies.
type ImportOptions struct {
	Mode     string
	SkipDups bool
}

// Importer reads and writes bookmark files.
type Importer interface {
	Parse(path string, lib *model.Library) (*ImportPlan, error)
	// Apply adds the checked folders to lib (main thread, inside
	// Store.Update) and returns the bookmarks added. Top-level folders
	// become groups, second-level sub-groups, deeper levels flatten into
	// "a / b" sub-groups; loose items go to a sub-group "未分组".
	Apply(lib *model.Library, plan *ImportPlan, opts ImportOptions) []string
	// Export writes the live library as "html" (Netscape) or "json".
	Export(lib *model.Library, path, format string) error
}
