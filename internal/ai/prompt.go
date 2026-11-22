package ai

import (
	"encoding/json"
	"errors"
	"strings"

	"cooool-mark/internal/app"
	"cooool-mark/internal/locales"
)

const (
	polishInstructionsZHNoGroup = `你是书签整理助手。根据提供的网页信息，输出简洁的中文标题(title)和一句话中文功能简介(note)。
只输出严格 JSON（不要 markdown 代码块或额外说明）：{"title":"...","note":"..."}
规则：
- title：保留原产品名、品牌名或项目名（英文专有名词不翻译），去掉"- 官网"、"_哔哩哔哩"等冗余后缀，不超过20字。
- note：用一句简短中文（10-25字）说明这个网页本身的用途或内容；不要重复标题，不要写"该页面属于..."或任何分组说明；信息不足时输出""。
示例：
输入：网址: https://github.com/junegunn/fzf | 原标题: GitHub - junegunn/fzf: A command-line fuzzy finder | 摘要: General-purpose command-line fuzzy finder.
输出：{"title":"fzf","note":"通用的命令行模糊搜索工具"}
输入：网址: https://excalidraw.com/ | 原标题: Excalidraw | Hand-drawn look & feel
输出：{"title":"Excalidraw","note":"开源的手绘风格在线协作白板工具"}`

	polishInstructionsZHWithGroup = `你是书签整理助手。根据提供的网页信息，输出简洁的中文标题(title)、一句话中文功能简介(note)和最匹配分组(group)。
只输出严格 JSON（不要 markdown 代码块或额外说明）：{"title":"...","note":"...","group":"..."}
规则：
- title：保留原产品名、品牌名或项目名（英文专有名词不翻译），去掉"- 官网"、"_哔哩哔哩"等冗余后缀，不超过20字。
- note：用一句简短中文（10-25字）说明这个网页本身的用途或内容。严禁在 note 中写分类理由、分组说明或"该页面属于..."；信息不足时输出""。
- group：必须完全等于"可选分组"中的某一项；都不合适就输出""。
示例：
输入：网址: https://github.com/junegunn/fzf | 原标题: GitHub - junegunn/fzf: A command-line fuzzy finder | 可选分组: 开发 / 工具 | 设计 / 灵感
输出：{"title":"fzf","note":"通用的命令行模糊搜索工具","group":"开发 / 工具"}
输入：网址: https://excalidraw.com/ | 原标题: Excalidraw | Hand-drawn look & feel | 可选分组: 开发 / 工具 | 影音 / 娱乐
输出：{"title":"Excalidraw","note":"开源的手绘风格在线协作白板工具","group":""}`

	polishInstructionsENNoGroup = `You are a bookmark organizer. Given webpage details, output a concise title and a one-sentence description of what the webpage does.
Output strict JSON only (no markdown fences or extra text): {"title":"...","note":"..."}
Rules:
- title: keep the core product, brand, or project name; strip redundant site suffixes like " - Official Site"; concise and clear.
- note: one short sentence (under 20 words) describing what the site or page is for. Never repeat the title, never mention grouping or classification; output "" if unclear.
Examples:
Input: URL: https://github.com/junegunn/fzf | Original title: GitHub - junegunn/fzf: A command-line fuzzy finder
Output: {"title":"fzf","note":"General-purpose command-line fuzzy finder"}
Input: URL: https://excalidraw.com/ | Original title: Excalidraw | Hand-drawn look & feel
Output: {"title":"Excalidraw","note":"Open-source hand-drawn style collaborative whiteboard"}`

	polishInstructionsENWithGroup = `You are a bookmark organizer. Given webpage details, output a concise title, a one-sentence description of what the webpage does, and the best matching group.
Output strict JSON only (no markdown fences or extra text): {"title":"...","note":"...","group":"..."}
Rules:
- title: keep the core product, brand, or project name; strip redundant site suffixes like " - Official Site".
- note: one short sentence (under 20 words) describing what the site or page itself does. Never write grouping rationale or mention groups in note; output "" if unclear.
- group: must exactly match one of the available groups, or "" if none fit.
Examples:
Input: URL: https://github.com/junegunn/fzf | Original title: GitHub - junegunn/fzf: A command-line fuzzy finder | Available groups: Dev / Tools | Design / Inspiration
Output: {"title":"fzf","note":"General-purpose command-line fuzzy finder","group":"Dev / Tools"}
Input: URL: https://excalidraw.com/ | Original title: Excalidraw | Hand-drawn look & feel | Available groups: Dev / Tools | Media / Video
Output: {"title":"Excalidraw","note":"Open-source hand-drawn style collaborative whiteboard","group":""}`
)

func polishJSONSchema(withGroup bool) map[string]any {
	props := map[string]any{
		"title": map[string]any{"type": "string"},
		"note":  map[string]any{"type": "string"},
	}
	required := []string{"title", "note"}
	if withGroup {
		props["group"] = map[string]any{"type": "string"}
		required = append(required, "group")
	}
	return map[string]any{
		"type": "json_schema",
		"json_schema": map[string]any{
			"name":   "bookmark_polish",
			"strict": true,
			"schema": map[string]any{
				"type":                 "object",
				"properties":           props,
				"required":             required,
				"additionalProperties": false,
			},
		},
	}
}

func buildPolishPrompt(in app.PolishInput) (instructions, userPrompt string) {
	withGroup := len(in.Choices) > 0
	if locales.IsChinese() {
		var b strings.Builder
		b.WriteString("网址: ")
		b.WriteString(strings.TrimSpace(in.URL))
		if t := strings.TrimSpace(in.Title); t != "" {
			b.WriteString("\n原标题: ")
			b.WriteString(truncateRunes(t, 160))
		}
		if d := strings.TrimSpace(in.Description); d != "" && !app.IsJunkAINote(d) {
			b.WriteString("\n摘要: ")
			b.WriteString(truncateRunes(d, 300))
		}
		if withGroup {
			b.WriteString("\n可选分组: ")
			for i, c := range in.Choices {
				if i > 0 {
					b.WriteString(" | ")
				}
				b.WriteString(c.Path)
			}
			return polishInstructionsZHWithGroup, b.String()
		}
		return polishInstructionsZHNoGroup, b.String()
	}

	var b strings.Builder
	b.WriteString("URL: ")
	b.WriteString(strings.TrimSpace(in.URL))
	if t := strings.TrimSpace(in.Title); t != "" {
		b.WriteString("\nOriginal title: ")
		b.WriteString(truncateRunes(t, 160))
	}
	if d := strings.TrimSpace(in.Description); d != "" && !app.IsJunkAINote(d) {
		b.WriteString("\nSummary: ")
		b.WriteString(truncateRunes(d, 300))
	}
	if withGroup {
		b.WriteString("\nAvailable groups: ")
		for i, c := range in.Choices {
			if i > 0 {
				b.WriteString(" | ")
			}
			b.WriteString(c.Path)
		}
		return polishInstructionsENWithGroup, b.String()
	}
	return polishInstructionsENNoGroup, b.String()
}

type rawPolishJSON struct {
	Title string `json:"title"`
	Note  string `json:"note"`
	Group string `json:"group"`
}

func parsePolishJSON(raw string, in app.PolishInput) (app.Polish, error) {
	cleaned := stripThinkingAndFences(raw)
	jsonText, ok := extractJSONObject(cleaned)
	if !ok {
		return app.Polish{}, errors.New(locales.AiErrNoJSON())
	}

	var parsed rawPolishJSON
	if err := json.Unmarshal([]byte(jsonText), &parsed); err != nil {
		return app.Polish{}, errors.New(locales.AiErrParseJSON(err.Error()))
	}

	out := app.Polish{
		Title: cleanLine(parsed.Title),
		Note:  cleanLine(parsed.Note),
	}

	if len(in.Choices) > 0 {
		if g := strings.TrimSpace(parsed.Group); g != "" {
			gid, sid := matchGroupChoice(g, in.Choices)
			out.GroupID = gid
			out.SubID = sid
		}
	}
	return sanitizePolish(out, in), nil
}

func cleanLine(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	return strings.Join(strings.Fields(s), " ")
}

func sanitizePolish(out app.Polish, in app.PolishInput) app.Polish {
	out.Title = strings.TrimRight(strings.TrimSpace(out.Title), "。.")
	if isJunkTitle(out.Title) {
		out.Title = ""
	}

	note := strings.TrimSpace(out.Note)
	if note != "" {
		if app.IsJunkAINote(note) ||
			strings.EqualFold(note, out.Title) ||
			strings.EqualFold(note, strings.TrimSpace(in.URL)) {
			note = ""
		}
	}
	out.Note = note
	return out
}

func isJunkTitle(title string) bool {
	s := strings.ToLower(strings.TrimSpace(title))
	switch s {
	case "", "无标题", "未知标题", "未知页面", "精简中文标题", "untitled", "unknown", "concise title":
		return true
	}
	return false
}

func matchGroupChoice(groupPath string, choices []app.GroupChoice) (string, string) {
	if groupPath == "" || len(choices) == 0 {
		return "", ""
	}
	for _, c := range choices {
		if c.Path == groupPath {
			return c.GroupID, c.SubID
		}
	}
	normTarget := normalizeChoicePath(groupPath)
	for _, c := range choices {
		if normalizeChoicePath(c.Path) == normTarget {
			return c.GroupID, c.SubID
		}
	}
	return "", ""
}

func normalizeChoicePath(s string) string {
	parts := strings.Split(s, "/")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return strings.ToLower(strings.Join(parts, "/"))
}

func stripThinkingAndFences(s string) string {
	s = strings.TrimSpace(s)
	for {
		start := strings.Index(s, "<think>")
		if start < 0 {
			break
		}
		end := strings.Index(s[start:], "</think>")
		if end < 0 {
			s = strings.TrimSpace(s[:start])
			break
		}
		s = strings.TrimSpace(s[:start] + s[start+end+len("</think>"):])
	}
	if strings.HasPrefix(s, "```") {
		if nl := strings.IndexByte(s, '\n'); nl >= 0 {
			s = s[nl+1:]
		}
		if end := strings.LastIndex(s, "```"); end >= 0 {
			s = s[:end]
		}
	}
	return strings.TrimSpace(s)
}

func extractJSONObject(s string) (string, bool) {
	start := strings.IndexByte(s, '{')
	if start < 0 {
		return "", false
	}
	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(s); i++ {
		c := s[i]
		if escaped {
			escaped = false
			continue
		}
		if inString {
			if c == '\\' {
				escaped = true
			} else if c == '"' {
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[start : i+1], true
			}
		}
	}
	return "", false
}

func truncateRunes(s string, maxRunes int) string {
	rs := []rune(s)
	if len(rs) <= maxRunes {
		return s
	}
	return string(rs[:maxRunes]) + "…"
}
