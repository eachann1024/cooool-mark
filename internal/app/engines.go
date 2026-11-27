package app

import "cooool-mark/internal/model"

// Search engines: template bookmarks (URL contains {query}) double as engines.
//
// Every search box (main window search, tray / edge / notch search, the global
// quick-search hotkey) shows bookmark hits first, then one "search <query> with
// <engine>" row per entry of Engines(), for ANY non-empty input. Typing
// "<keyword> " locks that engine (plan B); Enter then opens it with the rest.
//
// *App implements SearchEngines in engines_impl.go:
//
//	var _ SearchEngines = (*App)(nil)
type SearchEngines interface {
	// Engines returns the templates shown under every search: IsTemplate, not
	// in trash, SearchOff false; ordered by SearchRank (>0, ascending) first,
	// then the rest in library order (group, sub-group, Order).
	Engines() []*model.Bookmark
	// AllTemplates is every live template in the same order, SearchOff ones
	// included (the settings list).
	AllTemplates() []*model.Bookmark
	// EngineByKeyword matches kw case-insensitively against Keywords; nil if none.
	EngineByKeyword(kw string) *model.Bookmark
	// ParseLock reports whether input starts with "<keyword> " for a known
	// keyword; rest is everything after that first space (may be empty).
	ParseLock(input string) (b *model.Bookmark, rest string, ok bool)
	// KeywordOwner returns the live bookmark other than exceptID that already
	// uses kw, or nil.
	KeywordOwner(kw, exceptID string) *model.Bookmark
	// SetKeywords stores NormalizeKeywords(kws), bumps UpdatedAt and saves.
	SetKeywords(id string, kws []string)
	// SetEngineVisible sets SearchOff = !on, bumps UpdatedAt and saves.
	SetEngineVisible(id string, on bool)
	// MoveEngine moves a template up (delta < 0) or down in AllTemplates()
	// order (hidden ones included) by rewriting SearchRank of every template
	// to 1..n in the new order, then saves. Out-of-range moves are no-ops.
	MoveEngine(id string, delta int)
}

// SearchParam is the query value found in a search-results URL:
// Pre + Value + Post == the original URL.
type SearchParam struct {
	Pre, Value, Post string
}

// Package-level helpers implemented in engines_impl.go (contract only):
//
//	// DetectSearchParam finds the first non-empty query parameter named one
//	// of q, query, wd, word, keyword, keywords, search_text, search, s, k, kw,
//	// text. ok is false for template URLs or when none is found.
//	func DetectSearchParam(rawURL string) (p SearchParam, ok bool)
//
//	// NormalizeKeywords trims, lower-cases, drops empty / space-containing
//	// words and duplicates, keeping order.
//	func NormalizeKeywords(kws []string) []string
//
//	// SuggestKeywords proposes trigger words for a template from its title and
//	// host: a 2-character CJK title prefix, the domain stem (douban), and its
//	// initials when known (db); at most 3, normalized. Callers filter out
//	// words already taken (KeywordOwner).
//	func SuggestKeywords(title, rawURL string) []string
