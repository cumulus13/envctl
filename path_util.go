package main

import "strings"

// lowerASCII is a tiny ASCII-only lowercase helper, kept dependency-free
// and platform-neutral so it's usable (and directly testable) from both
// windows-only files and plain files like this one.
func lowerASCII(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}

// pathContainsDir reports whether dir is already one of the ';'-separated
// entries in pathValue. Comparison is case-insensitive and ignores a
// trailing backslash, since Windows paths are case-insensitive and
// "C:\foo" and "C:\foo\" refer to the same directory.
func pathContainsDir(pathValue, dir string) bool {
	target := normalizeForCompare(dir)
	for _, entry := range strings.Split(pathValue, ";") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if normalizeForCompare(entry) == target {
			return true
		}
	}
	return false
}

func normalizeForCompare(p string) string {
	p = strings.TrimSpace(p)
	p = strings.TrimSuffix(p, "\\")
	return lowerASCII(p)
}

// appendToPath returns pathValue with dir appended, if it isn't already
// present (case-insensitively). alreadyPresent reports which case
// happened, so the caller can decide whether a registry write is needed
// at all.
func appendToPath(pathValue, dir string) (newPath string, alreadyPresent bool) {
	if pathContainsDir(pathValue, dir) {
		return pathValue, true
	}
	trimmed := strings.TrimRight(strings.TrimSpace(pathValue), ";")
	if trimmed == "" {
		return dir, false
	}
	return trimmed + ";" + dir, false
}
