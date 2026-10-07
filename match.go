package main

import (
	"fmt"
	"regexp"
	"strings"
)

// compilePattern builds a case-insensitive-capable matcher from either a
// shell-style wildcard (* and ?) or a raw regex.
func compilePattern(pattern string, isRegex bool, caseSensitive bool) (*regexp.Regexp, error) {
	var exprBody string
	if isRegex {
		exprBody = pattern
	} else {
		exprBody = globToRegex(pattern)
	}
	prefix := ""
	if !caseSensitive {
		prefix = "(?i)"
	}
	re, err := regexp.Compile(prefix + exprBody)
	if err != nil {
		return nil, fmt.Errorf("invalid pattern %q: %w", pattern, err)
	}
	return re, nil
}

// globToRegex converts a wildcard pattern (* = any run of chars, ? = single
// char) into an equivalent regexp, escaping everything else. The result is
// NOT anchored, so "SDK" matches anywhere, "SDK*" matches a prefix, etc. —
// anchor explicitly with ^...$ in the pattern if you want a full match.
func globToRegex(glob string) string {
	var b strings.Builder
	for _, r := range glob {
		switch r {
		case '*':
			b.WriteString(".*")
		case '?':
			b.WriteString(".")
		default:
			b.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	return b.String()
}
