package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Config holds user-customizable hex colors and emoji icons.
// Colors are given as hex strings like "#FFFF00" — never RGB tuples —
// matching what was asked for.
type Config struct {
	Colors  ColorConfig `json:"colors"`
	Icons   IconConfig  `json:"icons"`
	NoColor bool        `json:"no_color"`
	NoEmoji bool        `json:"no_emoji"`
}

type ColorConfig struct {
	Key            string `json:"key"`             // variable name
	Value          string `json:"value"`           // variable value
	SourceUser     string `json:"source_user"`     // USER scope tag
	SourceSystem   string `json:"source_system"`   // SYSTEM scope tag
	SourceCurrent  string `json:"source_current"`  // CURRENT (live) tag
	MatchHighlight string `json:"match_highlight"` // substring that matched a search
	Dim            string `json:"dim"`             // separators, punctuation
	Warning        string `json:"warning"`
	Error          string `json:"error"`
}

type IconConfig struct {
	Key     string `json:"key"`
	User    string `json:"user"`
	System  string `json:"system"`
	Current string `json:"current"`
	Match   string `json:"match"`
	Warning string `json:"warning"`
	Error   string `json:"error"`
	OK      string `json:"ok"`
}

func defaultConfig() Config {
	return Config{
		Colors: ColorConfig{
			Key:            "#00FFFF", // cyan
			Value:          "#FFFF00", // yellow
			SourceUser:     "#00FF88", // green
			SourceSystem:   "#FF8800", // orange
			SourceCurrent:  "#AAAAAA", // grey
			MatchHighlight: "#FF00FF", // magenta
			Dim:            "#666666",
			Warning:        "#FFAA00",
			Error:          "#FF3333",
		},
		Icons: IconConfig{
			Key:     "🔑",
			User:    "👤",
			System:  "🖥️",
			Current: "⚡",
			Match:   "🔍",
			Warning: "⚠️",
			Error:   "❌",
			OK:      "✅",
		},
	}
}

func defaultConfigPath() string {
	if appData := os.Getenv("APPDATA"); appData != "" {
		return filepath.Join(appData, "envctl", "config.json")
	}
	return "envctl-config.json"
}

// loadConfig reads path if it exists, else returns defaults untouched.
// Any field left blank/zero in the file falls back to the default so a
// partial config file (e.g. only overriding "value") works fine.
func loadConfig(path string) (Config, error) {
	cfg := defaultConfig()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return cfg, nil
	}
	if err != nil {
		return cfg, fmt.Errorf("reading config %s: %w", path, err)
	}
	var fileCfg Config
	if err := json.Unmarshal(data, &fileCfg); err != nil {
		return cfg, fmt.Errorf("parsing config %s: %w", path, err)
	}
	mergeNonEmpty(&cfg, &fileCfg)
	return cfg, nil
}

func mergeNonEmpty(base *Config, override *Config) {
	bc, oc := &base.Colors, &override.Colors
	strs := []*string{
		&bc.Key, &bc.Value, &bc.SourceUser, &bc.SourceSystem, &bc.SourceCurrent,
		&bc.MatchHighlight, &bc.Dim, &bc.Warning, &bc.Error,
	}
	ostrs := []string{
		oc.Key, oc.Value, oc.SourceUser, oc.SourceSystem, oc.SourceCurrent,
		oc.MatchHighlight, oc.Dim, oc.Warning, oc.Error,
	}
	for i, p := range strs {
		if ostrs[i] != "" {
			*p = ostrs[i]
		}
	}
	bi, oi := &base.Icons, &override.Icons
	istrs := []*string{&bi.Key, &bi.User, &bi.System, &bi.Current, &bi.Match, &bi.Warning, &bi.Error, &bi.OK}
	oistrs := []string{oi.Key, oi.User, oi.System, oi.Current, oi.Match, oi.Warning, oi.Error, oi.OK}
	for i, p := range istrs {
		if oistrs[i] != "" {
			*p = oistrs[i]
		}
	}
	if override.NoColor {
		base.NoColor = true
	}
	if override.NoEmoji {
		base.NoEmoji = true
	}
}

func writeDefaultConfig(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil && filepath.Dir(path) != "." {
		return err
	}
	data, err := json.MarshalIndent(defaultConfig(), "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// hexToANSI converts "#RRGGBB" (or "RRGGBB") into a 24-bit ANSI foreground
// escape sequence. Hex in, not RGB tuples, as requested.
func hexToANSI(hex string) (string, error) {
	h := strings.TrimPrefix(strings.TrimSpace(hex), "#")
	if len(h) != 6 {
		return "", fmt.Errorf("invalid hex color %q: expected #RRGGBB", hex)
	}
	r, err := strconv.ParseUint(h[0:2], 16, 8)
	if err != nil {
		return "", fmt.Errorf("invalid hex color %q: %w", hex, err)
	}
	g, err := strconv.ParseUint(h[2:4], 16, 8)
	if err != nil {
		return "", fmt.Errorf("invalid hex color %q: %w", hex, err)
	}
	b, err := strconv.ParseUint(h[4:6], 16, 8)
	if err != nil {
		return "", fmt.Errorf("invalid hex color %q: %w", hex, err)
	}
	return fmt.Sprintf("\x1b[38;2;%d;%d;%dm", r, g, b), nil
}

const ansiReset = "\x1b[0m"

// painter wraps text in a hex color's ANSI codes, or returns it unmodified
// when colors are disabled (NO_COLOR env var, --no-color flag, non-tty).
type painter struct {
	enabled bool
}

func (p painter) paint(hex, text string) string {
	if !p.enabled || text == "" {
		return text
	}
	code, err := hexToANSI(hex)
	if err != nil {
		return text // bad config value: fail soft, never crash the CLI over color
	}
	return code + text + ansiReset
}
