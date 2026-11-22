package ai

import (
	"encoding/json"
	"errors"
	"strings"

	"cooool-mark/internal/app"
	"cooool-mark/internal/locales"
)

const (
	polishInstructionsZH = `你是书签整理助手。请根据网页信息生成简洁准确的中文整理结果，只输出严格 JSON（不要 markdown 代码块或额外说明），格式：{"title":"精简中文标题","note":"一句话中文备注","group":"最匹配的分组路径或空字符串"}`
	polishInstructionsEN = `You are a bookmark organizer. Given webpage details, produce concise and accurate English metadata as strict JSON only (no markdown fences or extra text) in this format: {"title":"concise title","note":"one-sentence note","group":"best matching group path or empty string"}`
)

func buildPolishPrompt(in app.PolishInput) (instructions, userPrompt string) {
	if locales.IsChinese() {
		var b strings.Builder
		b.WriteString("网址: ")
		b.WriteString(strings.TrimSpace(in.URL))
		if t := strings.TrimSpace(in.Title); t != "" {
			b.WriteString("\n原标题: ")
			b.WriteString(truncateRunes(t, 160))
		}
		if d := strings.TrimSpace(in.Description); d != "" {
			b.WriteString("\n简介: ")
			b.WriteString(truncateRunes(d, 300))
		}
		if len(in.Choices) > 0 {
			b.WriteString("\n可选分组: ")
			for i, c := range in.Choices {
				if i > 0 {
					b.WriteString(" | ")
				}
				b.WriteString(c.Path)
			}
		}
		b.WriteString("\n要求: 标题精简并去掉网站后缀；备注限一句话；group 必须完全等于某个可选分组或留空\"\"。")
		return polishInstructionsZH, b.String()
	}

	var b strings.Builder
	b.WriteString("URL: ")
	b.WriteString(strings.TrimSpace(in.URL))
	if t := strings.TrimSpace(in.Title); t != "" {
		b.WriteString("\nOriginal title: ")
		b.WriteString(truncateRunes(t, 160))
	}
	if d := strings.TrimSpace(in.Description); d != "" {
		b.WriteString("\nDescription: ")
		b.WriteString(truncateRunes(d, 300))
	}
	if len(in.Choices) > 0 {
		b.WriteString("\nAvailable groups: ")
		for i, c := range in.Choices {
			if i > 0 {
				b.WriteString(" | ")
			}
			b.WriteString(c.Path)
		}
	}
	b.WriteString("\nRules: keep title concise without redundant site suffix; note is one sentence; group must exactly match one of the available groups or \"\".")
	return polishInstructionsEN, b.String()
}

type rawPolishJSON struct {
	Title string `json:"title"`
	Note  string `json:"note"`
	Group string `json:"group"`
}

func parsePolishJSON(raw string, choices []app.GroupChoice) (app.Polish, error) {
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
		Title: strings.TrimSpace(parsed.Title),
		Note:  strings.TrimSpace(parsed.Note),
	}

	if g := strings.TrimSpace(parsed.Group); g != "" {
		gid, sid := matchGroupChoice(g, choices)
		out.GroupID = gid
		out.SubID = sid
	}
	return out, nil
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
