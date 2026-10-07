package main

import "testing"

func TestPathContainsDir(t *testing.T) {
	cases := []struct {
		name      string
		pathValue string
		dir       string
		want      bool
	}{
		{"exact match", `C:\tools\a;C:\tools\b`, `C:\tools\b`, true},
		{"case-insensitive", `C:\Tools\A;C:\Tools\B`, `c:\tools\b`, true},
		{"trailing backslash ignored", `C:\tools\a;C:\tools\b\`, `C:\tools\b`, true},
		{"trailing backslash on query side", `C:\tools\a;C:\tools\b`, `C:\tools\b\`, true},
		{"not present", `C:\tools\a;C:\tools\b`, `C:\tools\c`, false},
		{"empty path value", ``, `C:\tools\a`, false},
		{"stray semicolons/whitespace", ` ; C:\tools\a ; ;C:\tools\b ;`, `C:\tools\b`, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := pathContainsDir(c.pathValue, c.dir)
			if got != c.want {
				t.Errorf("pathContainsDir(%q, %q) = %v, want %v", c.pathValue, c.dir, got, c.want)
			}
		})
	}
}

func TestAppendToPath(t *testing.T) {
	cases := []struct {
		name              string
		pathValue         string
		dir               string
		wantPath          string
		wantAlreadyExists bool
	}{
		{
			name:              "append to existing path",
			pathValue:         `C:\tools\a;C:\tools\b`,
			dir:               `C:\new`,
			wantPath:          `C:\tools\a;C:\tools\b;C:\new`,
			wantAlreadyExists: false,
		},
		{
			name:              "empty existing path",
			pathValue:         ``,
			dir:               `C:\new`,
			wantPath:          `C:\new`,
			wantAlreadyExists: false,
		},
		{
			name:              "already present — unchanged, flagged",
			pathValue:         `C:\tools\a;C:\new`,
			dir:               `C:\new`,
			wantPath:          `C:\tools\a;C:\new`,
			wantAlreadyExists: true,
		},
		{
			name:              "trailing semicolon on existing path handled",
			pathValue:         `C:\tools\a;C:\tools\b;`,
			dir:               `C:\new`,
			wantPath:          `C:\tools\a;C:\tools\b;C:\new`,
			wantAlreadyExists: false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotPath, gotAlready := appendToPath(c.pathValue, c.dir)
			if gotPath != c.wantPath || gotAlready != c.wantAlreadyExists {
				t.Errorf("appendToPath(%q, %q) = (%q, %v), want (%q, %v)",
					c.pathValue, c.dir, gotPath, gotAlready, c.wantPath, c.wantAlreadyExists)
			}
		})
	}
}
