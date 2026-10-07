package main

import (
	"fmt"
	"regexp"
	"strings"
)

var ansiEscapeRE = regexp.MustCompile("\x1b\\[[0-9;]*m")

// visibleLen returns the rune length of s ignoring ANSI escape sequences,
// so column padding lines up even after text has been colorized.
func visibleLen(s string) int {
	stripped := ansiEscapeRE.ReplaceAllString(s, "")
	return len([]rune(stripped))
}

// padRight pads s (which may already contain ANSI codes) to width columns
// of VISIBLE text, never counting escape-code bytes as width.
func padRight(s string, width int) string {
	gap := width - visibleLen(s)
	if gap <= 0 {
		return s
	}
	return s + strings.Repeat(" ", gap)
}

// row is one renderable line: icon, scope tag, key, value — each already
// colorized (or plain, if colors are off). highlightInValue marks which
// value substrings to additionally wrap in the match color.
type row struct {
	Icon     string
	ScopeTag string
	Key      string
	Value    string
}

// renderTable prints rows as aligned columns with two-space gutters and
// NO border/box-drawing characters at all, per spec.
func renderTable(rows []row) {
	if len(rows) == 0 {
		fmt.Println("(no matching environment variables)")
		return
	}
	scopeW, keyW := 0, 0
	for _, r := range rows {
		if w := visibleLen(r.ScopeTag); w > scopeW {
			scopeW = w
		}
		if w := visibleLen(r.Key); w > keyW {
			keyW = w
		}
	}
	for _, r := range rows {
		fmt.Printf("%s %s  %s = %s\n",
			r.Icon,
			padRight(r.ScopeTag, scopeW),
			padRight(r.Key, keyW),
			r.Value,
		)
	}
}

// highlightMatches wraps every substring of text matched by re in the given
// hex color (via p), leaving the rest of the text untouched/uncolored so
// only the actual hit stands out.
func highlightMatches(p painter, hex string, text string, re *regexp.Regexp) string {
	if !p.enabled {
		return text
	}
	locs := re.FindAllStringIndex(text, -1)
	if locs == nil {
		return text
	}
	var b strings.Builder
	last := 0
	for _, loc := range locs {
		b.WriteString(text[last:loc[0]])
		b.WriteString(p.paint(hex, text[loc[0]:loc[1]]))
		last = loc[1]
	}
	b.WriteString(text[last:])
	return b.String()
}
