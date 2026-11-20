// Package search finds bookmarks by title, URL, note, tags, and Chinese pinyin.
package search

import (
	"sort"
	"strings"
	"sync"
	"unicode"

	"github.com/mozillazg/go-pinyin"

	"cooool-mark/internal/app"
	"cooool-mark/internal/locales"
	"cooool-mark/internal/model"
)

type titleUnit struct {
	lower   rune
	isHan   bool
	isSep   bool
	pinyins []string // lowercased normal pinyins (without tone), non-empty when isHan
}

type textIndex struct {
	raw   string
	lower string
	runes []rune
	units []titleUnit
	mask  uint64
}

type bmIndex struct {
	title    textIndex
	urlLower string
	rawURL   string
	note     textIndex
	tagsKey  string
	tags     []textIndex
}

type searcher struct {
	mu      sync.Mutex
	cache   map[string]*bmIndex
	hanDict map[rune][]string
}

// New returns the Searcher.
func New() app.Searcher {
	return &searcher{
		cache:   make(map[string]*bmIndex),
		hanDict: make(map[rune][]string, 4096),
	}
}

// Search matches live bookmarks by multi-term AND across title, URL, note and tags,
// supporting Chinese full pinyin and initials, and maps matched title pinyin back to rune ranges.
func (s *searcher) Search(lib *model.Library, query string, groupID string) []app.Hit {
	if lib == nil {
		return nil
	}
	terms := strings.Fields(strings.ToLower(strings.TrimSpace(query)))
	if len(terms) == 0 {
		return nil
	}

	type parsedTerm struct {
		raw       string
		runes     []rune
		isASCII   bool
		hasAlnum  bool
		asciiMask uint64
	}
	pterms := make([]parsedTerm, len(terms))
	for i, t := range terms {
		rs := []rune(t)
		isASCII, hasAlnum, mask := analyzeTerm(t)
		pterms[i] = parsedTerm{
			raw:       t,
			runes:     rs,
			isASCII:   isASCII,
			hasAlnum:  hasAlnum,
			asciiMask: mask,
		}
	}

	// Build O(1) "group / sub" lookup table for this library snapshot.
	paths := buildPathMap(lib)

	type scoredHit struct {
		hit   app.Hit
		score int
	}
	var results []scoredHit

	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range lib.Bookmarks {
		b := &lib.Bookmarks[i]
		if b.InTrash() {
			continue
		}
		if groupID != "" && b.GroupID != groupID {
			continue
		}

		idx := s.indexFor(b)

		totalScore := 0
		matchedAll := true
		var ranges [][2]int

		for _, pt := range pterms {
			bestScore := 0
			var termRanges [][2]int

			// 1. Title literal match (prefix > contains)
			if len(pt.runes) > 0 && len(idx.title.runes) >= len(pt.runes) {
				if pos := indexRunes(idx.title.runes, pt.runes); pos >= 0 {
					if pos == 0 {
						bestScore = 4000
					} else {
						bestScore = 3000 - min(pos, 200)
					}
					termRanges = appendAllRuneRanges(termRanges, idx.title.runes, pt.runes, pos)
				}
			}

			// 2. Title pinyin match (if ASCII alnum and bitmask passes)
			if bestScore == 0 && pt.isASCII && pt.hasAlnum && (idx.title.mask&pt.asciiMask) == pt.asciiMask {
				if startR, endR, fullChars, ok := matchPinyinUnits(idx.title.units, pt.raw); ok {
					sc := 2000
					if startR == 0 {
						sc += 200
					}
					if fullChars >= len(pt.raw) {
						sc += 100
					}
					bestScore = sc
					termRanges = append(termRanges, [2]int{startR, endR})
				}
			}

			// 3. Other fields: tags > URL > note (literal then pinyin)
			if bestScore == 0 {
				otherScore := 0
				for ti := range idx.tags {
					tIdx := &idx.tags[ti]
					if tIdx.lower == pt.raw {
						otherScore = max(otherScore, 1400)
					} else if strings.Contains(tIdx.lower, pt.raw) {
						otherScore = max(otherScore, 1300)
					} else if pt.isASCII && pt.hasAlnum && (tIdx.mask&pt.asciiMask) == pt.asciiMask {
						if _, _, _, ok := matchPinyinUnits(tIdx.units, pt.raw); ok {
							otherScore = max(otherScore, 1200)
						}
					}
				}
				if strings.Contains(idx.urlLower, pt.raw) {
					otherScore = max(otherScore, 1150)
				}
				if idx.note.lower != "" {
					if strings.Contains(idx.note.lower, pt.raw) {
						otherScore = max(otherScore, 1100)
					} else if pt.isASCII && pt.hasAlnum && (idx.note.mask&pt.asciiMask) == pt.asciiMask {
						if _, _, _, ok := matchPinyinUnits(idx.note.units, pt.raw); ok {
							otherScore = max(otherScore, 1000)
						}
					}
				}
				bestScore = otherScore
			}

			if bestScore == 0 {
				matchedAll = false
				break
			}
			totalScore += bestScore
			if len(termRanges) > 0 {
				ranges = append(ranges, termRanges...)
			}
		}

		if !matchedAll {
			continue
		}

		// Average score across terms so single-term and multi-term keep tier separation.
		avgScore := totalScore / len(pterms)

		path := paths[groupSubKey{b.GroupID, b.SubID}]
		if path == "" {
			path = locales.Ungrouped()
		}

		results = append(results, scoredHit{
			hit: app.Hit{
				B:      b,
				Ranges: mergeRanges(ranges),
				Path:   path,
			},
			score: avgScore,
		})
	}

	sort.SliceStable(results, func(i, j int) bool {
		if results[i].score != results[j].score {
			return results[i].score > results[j].score
		}
		if results[i].hit.B.Order != results[j].hit.B.Order {
			return results[i].hit.B.Order < results[j].hit.B.Order
		}
		return results[i].hit.B.Title < results[j].hit.B.Title
	})

	out := make([]app.Hit, len(results))
	for i := range results {
		out[i] = results[i].hit
	}
	return out
}

type groupSubKey struct {
	g, s string
}

func buildPathMap(lib *model.Library) map[groupSubKey]string {
	m := make(map[groupSubKey]string, len(lib.Groups)*4)
	for i := range lib.Groups {
		g := &lib.Groups[i]
		for j := range g.Subs {
			sub := &g.Subs[j]
			m[groupSubKey{g.ID, sub.ID}] = g.Name + " / " + sub.Name
		}
	}
	return m
}

func (s *searcher) indexFor(b *model.Bookmark) *bmIndex {
	idx, ok := s.cache[b.ID]
	if !ok {
		idx = &bmIndex{}
		s.cache[b.ID] = idx
	}
	if !ok || idx.title.raw != b.Title {
		idx.title = s.buildTextIndex(b.Title, true)
	}
	if !ok || idx.rawURL != b.URL {
		idx.rawURL = b.URL
		idx.urlLower = strings.ToLower(b.URL)
	}
	if !ok || idx.note.raw != b.Note {
		idx.note = s.buildTextIndex(b.Note, false)
	}
	tagsKey := strings.Join(b.Tags, "\x00")
	if !ok || idx.tagsKey != tagsKey {
		idx.tagsKey = tagsKey
		if len(b.Tags) == 0 {
			idx.tags = nil
		} else {
			idx.tags = make([]textIndex, len(b.Tags))
			for i, t := range b.Tags {
				idx.tags[i] = s.buildTextIndex(t, false)
			}
		}
	}
	return idx
}

func (s *searcher) buildTextIndex(raw string, needRunes bool) textIndex {
	if raw == "" {
		return textIndex{}
	}
	lowerStr := strings.ToLower(raw)
	rs := []rune(raw)
	lowerRunes := make([]rune, len(rs))
	units := make([]titleUnit, len(rs))
	var mask uint64

	for i, r := range rs {
		lr := unicode.ToLower(r)
		lowerRunes[i] = lr
		if pys := s.lookupPinyin(r); len(pys) > 0 {
			units[i] = titleUnit{
				lower:   lr,
				isHan:   true,
				pinyins: pys,
			}
			for _, py := range pys {
				for j := 0; j < len(py); j++ {
					mask |= charBit(py[j])
				}
			}
		} else if isASCIIAlnum(lr) {
			units[i] = titleUnit{lower: lr}
			mask |= charBit(byte(lr))
		} else {
			units[i] = titleUnit{lower: lr, isSep: true}
		}
	}

	var keptRunes []rune
	if needRunes {
		keptRunes = lowerRunes
	}
	return textIndex{
		raw:   raw,
		lower: lowerStr,
		runes: keptRunes,
		units: units,
		mask:  mask,
	}
}

var pinyinArgs = pinyin.Args{
	Style:     pinyin.Normal,
	Heteronym: true,
}

func (s *searcher) lookupPinyin(r rune) []string {
	if r < 0x3000 {
		return nil
	}
	if pys, ok := s.hanDict[r]; ok {
		return pys
	}
	pys := pinyin.SinglePinyin(r, pinyinArgs)
	if len(pys) == 0 {
		s.hanDict[r] = nil
		return nil
	}
	// Deduplicate and ensure lowercase.
	out := make([]string, 0, len(pys))
	for _, p := range pys {
		p = strings.ToLower(p)
		if p == "" {
			continue
		}
		dup := false
		for _, prev := range out {
			if prev == p {
				dup = true
				break
			}
		}
		if !dup {
			out = append(out, p)
		}
	}
	s.hanDict[r] = out
	return out
}

func analyzeTerm(t string) (isASCII bool, hasAlnum bool, mask uint64) {
	isASCII = true
	for i := 0; i < len(t); i++ {
		c := t[i]
		if c >= 0x80 {
			return false, false, 0
		}
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			hasAlnum = true
			mask |= charBit(c)
		} else {
			// Non-alnum ASCII punctuation (e.g. '-', '.', '/') cannot match pure pinyin stream.
			return false, false, 0
		}
	}
	return isASCII, hasAlnum, mask
}

func isASCIIAlnum(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
}

func charBit(c byte) uint64 {
	if c >= 'a' && c <= 'z' {
		return 1 << uint(c-'a')
	}
	if c >= '0' && c <= '9' {
		return 1 << uint(26+(c-'0'))
	}
	return 0
}

func indexRunes(haystack, needle []rune) int {
	n := len(needle)
	if n == 0 {
		return 0
	}
	if len(haystack) < n {
		return -1
	}
	first := needle[0]
	limit := len(haystack) - n
	for i := 0; i <= limit; i++ {
		if haystack[i] != first {
			continue
		}
		match := true
		for j := 1; j < n; j++ {
			if haystack[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

func appendAllRuneRanges(dst [][2]int, haystack, needle []rune, firstPos int) [][2]int {
	n := len(needle)
	dst = append(dst, [2]int{firstPos, firstPos + n})
	pos := firstPos + n
	for pos <= len(haystack)-n {
		rel := indexRunes(haystack[pos:], needle)
		if rel < 0 {
			break
		}
		start := pos + rel
		dst = append(dst, [2]int{start, start + n})
		pos = start + n
	}
	return dst
}

// matchPinyinUnits finds the earliest match of an ASCII alphanumeric term
// against titleUnits (supporting full pinyin, initials, and mixed ASCII+Han).
func matchPinyinUnits(units []titleUnit, term string) (startRune, endRune, fullChars int, ok bool) {
	for i := 0; i < len(units); i++ {
		if units[i].isSep {
			continue
		}
		if endPos, fc, matched := matchPinyinAt(units, i, term, 0); matched {
			return i, endPos, fc, true
		}
	}
	return 0, 0, 0, false
}

func matchPinyinAt(units []titleUnit, pos int, term string, ti int) (endPos, fullChars int, ok bool) {
	if ti == len(term) {
		return pos, 0, true
	}
	if ti > 0 {
		for pos < len(units) && units[pos].isSep {
			pos++
		}
	}
	if pos >= len(units) {
		return 0, 0, false
	}

	u := units[pos]
	if !u.isHan {
		if u.isSep || u.lower != rune(term[ti]) {
			return 0, 0, false
		}
		return matchPinyinAt(units, pos+1, term, ti+1)
	}

	rem := term[ti:]
	// 1. Full syllable match (highest preference so "shaoshupai" -> [0, 3)).
	for _, py := range u.pinyins {
		if len(rem) >= len(py) && strings.HasPrefix(rem, py) {
			if end, fc, matched := matchPinyinAt(units, pos+1, term, ti+len(py)); matched {
				return end, len(py) + fc, true
			}
		}
	}
	// 2. Initial letter match (e.g. "ssp" -> 少数派).
	var triedInitials uint32
	for _, py := range u.pinyins {
		bit := uint32(1) << uint(py[0]-'a')
		if triedInitials&bit != 0 {
			continue
		}
		triedInitials |= bit
		if rem[0] == py[0] {
			if end, fc, matched := matchPinyinAt(units, pos+1, term, ti+1); matched {
				return end, fc, true
			}
		}
	}
	// 3. Final syllable prefix (e.g. "shaos" -> 少(shao)+数(s), or "zh" -> 知(zh) when initials didn't match).
	if len(rem) > 1 {
		for _, py := range u.pinyins {
			if len(rem) < len(py) && strings.HasPrefix(py, rem) {
				return pos + 1, len(rem), true
			}
		}
	}
	return 0, 0, false
}

func mergeRanges(ranges [][2]int) [][2]int {
	if len(ranges) <= 1 {
		return ranges
	}
	sort.Slice(ranges, func(i, j int) bool {
		if ranges[i][0] != ranges[j][0] {
			return ranges[i][0] < ranges[j][0]
		}
		return ranges[i][1] < ranges[j][1]
	})
	out := ranges[:0]
	cur := ranges[0]
	for _, r := range ranges[1:] {
		if r[0] <= cur[1] {
			if r[1] > cur[1] {
				cur[1] = r[1]
			}
		} else {
			out = append(out, cur)
			cur = r
		}
	}
	out = append(out, cur)
	return out
}
