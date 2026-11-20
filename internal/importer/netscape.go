package importer

import (
	"bytes"
	"errors"
	"fmt"
	stdhtml "html"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"

	"cooool-mark/internal/app"
	"cooool-mark/internal/locales"
	"cooool-mark/internal/model"
)

func parseNetscapeHTML(data []byte) (*app.ImportPlan, error) {
	z := html.NewTokenizer(bytes.NewReader(data))
	root := &app.ImportFolder{Checked: true}
	stack := []*app.ImportFolder{root}

	var (
		seenRootDL    bool
		hasPending    bool
		pendingFolder string
		inH3          bool
		h3Text        strings.Builder
		inA           bool
		aHref         string
		aAddDate      time.Time
		aText         strings.Builder
	)

	flushA := func() {
		if !inA {
			return
		}
		inA = false
		href := strings.TrimSpace(aHref)
		if href != "" && !strings.HasPrefix(strings.ToLower(href), "place:") {
			title := strings.TrimSpace(aText.String())
			if title == "" {
				title = href
			}
			cur := stack[len(stack)-1]
			cur.Items = append(cur.Items, model.Bookmark{
				Title:     title,
				URL:       href,
				CreatedAt: aAddDate,
			})
		}
	}

	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			if err := z.Err(); err != io.EOF {
				return nil, err
			}
			break
		}

		switch tt {
		case html.StartTagToken, html.SelfClosingTagToken:
			tok := z.Token()
			switch tok.Data {
			case "dt":
				flushA()
			case "h3":
				flushA()
				inH3 = true
				h3Text.Reset()
			case "dl":
				flushA()
				if inH3 {
					inH3 = false
					pendingFolder = strings.TrimSpace(h3Text.String())
					hasPending = true
				}
				if !seenRootDL && !hasPending {
					seenRootDL = true
					continue
				}
				seenRootDL = true
				if hasPending {
					name := strings.TrimSpace(pendingFolder)
					if name == "" {
						name = locales.UntitledFolder()
					}
					f := &app.ImportFolder{Name: name, Checked: true}
					parent := stack[len(stack)-1]
					parent.Children = append(parent.Children, f)
					stack = append(stack, f)
					hasPending = false
					pendingFolder = ""
				} else {
					stack = append(stack, stack[len(stack)-1])
				}
			case "a":
				flushA()
				hasPending = false
				pendingFolder = ""
				inA = true
				aHref = ""
				aAddDate = time.Time{}
				aText.Reset()
				for _, attr := range tok.Attr {
					switch attr.Key {
					case "href":
						aHref = attr.Val
					case "add_date":
						aAddDate = parseAddDate(attr.Val)
					}
				}
				if tt == html.SelfClosingTagToken {
					inA = false
				}
			}

		case html.EndTagToken:
			tok := z.Token()
			switch tok.Data {
			case "h3":
				if inH3 {
					inH3 = false
					pendingFolder = strings.TrimSpace(h3Text.String())
					hasPending = true
				}
			case "dl":
				flushA()
				hasPending = false
				pendingFolder = ""
				if len(stack) > 1 {
					stack = stack[:len(stack)-1]
				}
			case "a":
				flushA()
			}

		case html.TextToken:
			if inH3 {
				h3Text.WriteString(z.Token().Data)
			} else if inA {
				aText.WriteString(z.Token().Data)
			}
		}
	}
	flushA()

	folders := root.Children
	if len(root.Items) > 0 {
		ungroupedName := locales.Ungrouped()
		var ungrouped *app.ImportFolder
		for _, f := range folders {
			if f.Name == ungroupedName {
				ungrouped = f
				break
			}
		}
		if ungrouped != nil {
			ungrouped.Items = append(ungrouped.Items, root.Items...)
		} else {
			folders = append(folders, &app.ImportFolder{
				Name:    ungroupedName,
				Items:   root.Items,
				Checked: true,
			})
		}
	}

	if !hasAnyBookmark(folders) {
		return nil, errors.New(locales.ImportErrNoBookmarks())
	}

	return &app.ImportPlan{
		Format:  "html",
		Folders: folders,
	}, nil
}

func hasAnyBookmark(folders []*app.ImportFolder) bool {
	for _, f := range folders {
		if len(f.Items) > 0 || hasAnyBookmark(f.Children) {
			return true
		}
	}
	return false
}

func parseAddDate(raw string) time.Time {
	v, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || v <= 0 {
		return time.Time{}
	}
	switch {
	case v > 1e15:
		return time.UnixMicro(v)
	case v > 1e12:
		return time.UnixMilli(v)
	default:
		return time.Unix(v, 0)
	}
}

func exportNetscapeHTML(lib *model.Library, path string) error {
	var b strings.Builder
	exportTitle := stdhtml.EscapeString(locales.HtmlExportTitle())
	b.WriteString("<!DOCTYPE NETSCAPE-Bookmark-file-1>\n")
	b.WriteString("<META HTTP-EQUIV=\"Content-Type\" CONTENT=\"text/html; charset=UTF-8\">\n")
	fmt.Fprintf(&b, "<TITLE>%s</TITLE>\n", exportTitle)
	fmt.Fprintf(&b, "<H1>%s</H1>\n", exportTitle)
	b.WriteString("<DL><p>\n")

	seen := map[string]bool{}
	for _, g := range lib.Groups {
		fmt.Fprintf(&b, "    <DT><H3 ADD_DATE=\"%d\">%s</H3>\n", unixTS(g.UpdatedAt), stdhtml.EscapeString(g.Name))
		b.WriteString("    <DL><p>\n")
		for _, s := range g.Subs {
			fmt.Fprintf(&b, "        <DT><H3 ADD_DATE=\"%d\">%s</H3>\n", unixTS(s.UpdatedAt), stdhtml.EscapeString(s.Name))
			b.WriteString("        <DL><p>\n")
			for _, bm := range lib.In(g.ID, s.ID) {
				seen[bm.ID] = true
				title := bm.Title
				if title == "" {
					title = bm.URL
				}
				fmt.Fprintf(&b, "            <DT><A HREF=\"%s\" ADD_DATE=\"%d\">%s</A>\n",
					stdhtml.EscapeString(bm.URL), unixTS(bm.CreatedAt), stdhtml.EscapeString(title))
			}
			b.WriteString("        </DL><p>\n")
		}
		b.WriteString("    </DL><p>\n")
	}

	var orphans []*model.Bookmark
	for _, bm := range lib.Live() {
		if !seen[bm.ID] {
			orphans = append(orphans, bm)
		}
	}
	if len(orphans) > 0 {
		fmt.Fprintf(&b, "    <DT><H3>%s</H3>\n", stdhtml.EscapeString(locales.Ungrouped()))
		b.WriteString("    <DL><p>\n")
		for _, bm := range orphans {
			title := bm.Title
			if title == "" {
				title = bm.URL
			}
			fmt.Fprintf(&b, "        <DT><A HREF=\"%s\" ADD_DATE=\"%d\">%s</A>\n",
				stdhtml.EscapeString(bm.URL), unixTS(bm.CreatedAt), stdhtml.EscapeString(title))
		}
		b.WriteString("    </DL><p>\n")
	}

	b.WriteString("</DL><p>\n")
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func unixTS(t time.Time) int64 {
	if t.IsZero() {
		return time.Now().Unix()
	}
	return t.Unix()
}
