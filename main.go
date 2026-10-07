//go:build windows

package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// version is overridden at release-build time via:
//
//	-ldflags "-X main.version=vX.Y.Z"
//
// (see .github/workflows/release.yml). Must stay a var, not a const, for
// -ldflags -X to be able to set it.
var version = "dev"

func main() {
	enableANSI()

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "list", "ls":
		cmdList(os.Args[2:])
	case "find", "search":
		cmdFind(os.Args[2:])
	case "set":
		cmdSet(os.Args[2:])
	case "install":
		cmdInstall(os.Args[2:])
	case "config":
		cmdConfig(os.Args[2:])
	case "-h", "--help", "help":
		printUsage()
	case "-v", "--version", "version":
		fmt.Println("envctl", version)
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Print(`envctl - Windows environment variable inspector/editor

USAGE:
  envctl list   [--current | --user | --system] [--no-color] [--no-emoji] [--config PATH]
  envctl find   <pattern> [--regex] [--case-sensitive] [--keys-only] [--values-only]
                          [--current | --user | --system] [--no-color] [--no-emoji] [--config PATH]
  envctl set    <NAME> <VALUE> (--user | --system) [--type sz|expand] [--yes] [--dry-run]
  envctl install [--dir PATH] [--yes] [--dry-run]
  envctl config init [--config PATH]
  envctl version
  envctl help

SCOPE (list/find):
  (none)     permanent vars from BOTH the registry's USER and SYSTEM Environment keys
  --user     permanent USER vars only      (HKCU\Environment)
  --system   permanent SYSTEM vars only    (HKLM\...\Session Manager\Environment)
  --current  the LIVE environment of this terminal/process (what a child process
             launched right now would inherit) — not the registry at all

PATTERN (find):
  default: wildcard, * = any run of characters, ? = single character
  --regex: pattern is a Go-flavored regular expression instead

SET SAFETY:
  - Writes exactly ONE named registry value. Nothing else under Environment is
    ever touched, read-modified, or replaced as a block.
  - Shows the current value (if any) and asks for confirmation before writing,
    unless --yes is given.
  - --dry-run prints what would happen and writes nothing.
  - --system requires an elevated (Administrator) terminal.
  - After writing, a WM_SETTINGCHANGE broadcast is sent so NEW processes/shells
    pick it up. Already-open terminal windows will NOT see the change until you
    open a new one — that is a Windows limitation, not a bug in this tool.

INSTALL:
  Copies this running .exe to --dir (default %LOCALAPPDATA%\Programs\envctl)
  and adds that folder to your USER PATH — nothing else. No PowerShell, no
  separate installer script: it's the same tested registry code as "set",
  since PATH is just a normal value named "Path" under the same key.
  Never touches SYSTEM PATH, never needs Administrator.
  Shows exactly what it will do and asks [y/N] first, same as "set".

EXAMPLES:
  envctl list
  envctl list --current
  envctl find "JAVA*" --user
  envctl find "^GOPATH$" --regex
  envctl set MY_TOOL_HOME "C:\tools\mytool" --user
  envctl set JAVA_HOME "C:\jdk-21" --system --dry-run
  envctl install
  envctl install --dir "D:\tools\envctl" --dry-run
`)
}

// ---------- shared flag/scope handling ----------

type scopeFlags struct {
	current bool
	user    bool
	system  bool
}

func (s scopeFlags) validate() error {
	n := 0
	if s.current {
		n++
	}
	if s.user {
		n++
	}
	if s.system {
		n++
	}
	if n > 1 {
		return fmt.Errorf("--current, --user and --system are mutually exclusive")
	}
	return nil
}

func addScopeFlags(fs *flag.FlagSet, sf *scopeFlags) {
	fs.BoolVar(&sf.current, "current", false, "show the live environment of this terminal/process")
	fs.BoolVar(&sf.user, "user", false, "permanent USER scope only")
	fs.BoolVar(&sf.system, "system", false, "permanent SYSTEM scope only")
}

func loadCfgAndPainter(configPath string, noColor, noEmoji bool) (Config, painter) {
	path := configPath
	if path == "" {
		path = defaultConfigPath()
	}
	cfg, err := loadConfig(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: %v (using defaults)\n", err)
		cfg = defaultConfig()
	}
	if noColor {
		cfg.NoColor = true
	}
	if noEmoji {
		cfg.NoEmoji = true
	}
	p := painter{enabled: !cfg.NoColor}
	return cfg, p
}

// collectEntries gathers env vars according to scope flags, as a uniform
// list of (scope label, key, value) — "CURRENT" is a synthetic scope label
// for the live-process case.
type entry struct {
	ScopeLabel string // "USER", "SYSTEM", or "CURRENT"
	Key        string
	Value      string
}

func collectEntries(sf scopeFlags) ([]entry, error) {
	var out []entry
	switch {
	case sf.current:
		for _, kv := range os.Environ() {
			k, v, found := strings.Cut(kv, "=")
			if !found || k == "" {
				continue
			}
			out = append(out, entry{ScopeLabel: "CURRENT", Key: k, Value: v})
		}
	case sf.user:
		es, err := listPermanent(scopeUser)
		if err != nil {
			return nil, err
		}
		for _, e := range es {
			out = append(out, entry{ScopeLabel: "USER", Key: e.Name, Value: e.Value})
		}
	case sf.system:
		es, err := listPermanent(scopeSystem)
		if err != nil {
			return nil, err
		}
		for _, e := range es {
			out = append(out, entry{ScopeLabel: "SYSTEM", Key: e.Name, Value: e.Value})
		}
	default:
		eu, err := listPermanent(scopeUser)
		if err != nil {
			return nil, err
		}
		es, err := listPermanent(scopeSystem)
		if err != nil {
			return nil, err
		}
		for _, e := range eu {
			out = append(out, entry{ScopeLabel: "USER", Key: e.Name, Value: e.Value})
		}
		for _, e := range es {
			out = append(out, entry{ScopeLabel: "SYSTEM", Key: e.Name, Value: e.Value})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ScopeLabel != out[j].ScopeLabel {
			return out[i].ScopeLabel < out[j].ScopeLabel
		}
		return lowerASCII(out[i].Key) < lowerASCII(out[j].Key)
	})
	return out, nil
}

func scopeIcon(cfg Config, label string) string {
	if cfg.NoEmoji {
		return ""
	}
	switch label {
	case "USER":
		return cfg.Icons.User
	case "SYSTEM":
		return cfg.Icons.System
	default:
		return cfg.Icons.Current
	}
}

func scopeColor(cfg Config, label string) string {
	switch label {
	case "USER":
		return cfg.Colors.SourceUser
	case "SYSTEM":
		return cfg.Colors.SourceSystem
	default:
		return cfg.Colors.SourceCurrent
	}
}

// ---------- list ----------

func cmdList(args []string) {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	var sf scopeFlags
	addScopeFlags(fs, &sf)
	noColor := fs.Bool("no-color", false, "disable colored output")
	noEmoji := fs.Bool("no-emoji", false, "disable emoji icons")
	configPath := fs.String("config", "", "path to config file (default: %APPDATA%\\envctl\\config.json)")
	flagPart, _ := splitArgs(args, []string{"config"})
	fs.Parse(flagPart)

	if err := sf.validate(); err != nil {
		fatal(err)
	}
	cfg, p := loadCfgAndPainter(*configPath, *noColor, *noEmoji)

	entries, err := collectEntries(sf)
	if err != nil {
		fatal(err)
	}

	rows := make([]row, 0, len(entries))
	for _, e := range entries {
		rows = append(rows, row{
			Icon:     scopeIcon(cfg, e.ScopeLabel),
			ScopeTag: p.paint(scopeColor(cfg, e.ScopeLabel), e.ScopeLabel),
			Key:      p.paint(cfg.Colors.Key, e.Key),
			Value:    p.paint(cfg.Colors.Value, e.Value),
		})
	}
	renderTable(rows)
	fmt.Printf("\n%d variable(s)\n", len(entries))
}

// ---------- find / search ----------

func cmdFind(args []string) {
	fs := flag.NewFlagSet("find", flag.ExitOnError)
	var sf scopeFlags
	addScopeFlags(fs, &sf)
	isRegex := fs.Bool("regex", false, "treat pattern as a regular expression instead of a wildcard")
	caseSensitive := fs.Bool("case-sensitive", false, "match case-sensitively (default: case-insensitive)")
	keysOnly := fs.Bool("keys-only", false, "match against variable names only")
	valuesOnly := fs.Bool("values-only", false, "match against variable values only")
	noColor := fs.Bool("no-color", false, "disable colored output")
	noEmoji := fs.Bool("no-emoji", false, "disable emoji icons")
	configPath := fs.String("config", "", "path to config file")
	flagPart, positional := splitArgs(args, []string{"config"})
	fs.Parse(flagPart)

	if err := sf.validate(); err != nil {
		fatal(err)
	}
	if *keysOnly && *valuesOnly {
		fatal(fmt.Errorf("--keys-only and --values-only are mutually exclusive"))
	}
	if len(positional) != 1 {
		fatal(fmt.Errorf("expected exactly one pattern, got %d (quote it if it has spaces)", len(positional)))
	}
	pattern := positional[0]

	cfg, p := loadCfgAndPainter(*configPath, *noColor, *noEmoji)

	re, err := compilePattern(pattern, *isRegex, *caseSensitive)
	if err != nil {
		fatal(err)
	}

	entries, err := collectEntries(sf)
	if err != nil {
		fatal(err)
	}

	var rows []row
	for _, e := range entries {
		keyHit := !*valuesOnly && re.MatchString(e.Key)
		valHit := !*keysOnly && re.MatchString(e.Value)
		if !keyHit && !valHit {
			continue
		}
		keyText := e.Key
		valText := e.Value
		if keyHit {
			keyText = highlightMatches(p, cfg.Colors.MatchHighlight, e.Key, re)
		}
		if valHit {
			valText = highlightMatches(p, cfg.Colors.MatchHighlight, e.Value, re)
		}
		icon := scopeIcon(cfg, e.ScopeLabel)
		if !cfg.NoEmoji {
			icon = cfg.Icons.Match + icon
		}
		rows = append(rows, row{
			Icon:     icon,
			ScopeTag: p.paint(scopeColor(cfg, e.ScopeLabel), e.ScopeLabel),
			Key:      p.paint(cfg.Colors.Key, keyText),
			Value:    p.paint(cfg.Colors.Value, valText),
		})
	}
	renderTable(rows)
	fmt.Printf("\n%d match(es) for %q\n", len(rows), pattern)
}

// ---------- set ----------

func cmdSet(args []string) {
	fs := flag.NewFlagSet("set", flag.ExitOnError)
	user := fs.Bool("user", false, "write to the permanent USER environment")
	system := fs.Bool("system", false, "write to the permanent SYSTEM environment (requires Administrator)")
	typeOverride := fs.String("type", "", `registry type to use: "sz" or "expand" (default: auto-detect / preserve existing)`)
	yes := fs.Bool("yes", false, "skip the confirmation prompt")
	dryRun := fs.Bool("dry-run", false, "show what would happen, write nothing")
	flagPart, positional := splitArgs(args, []string{"type"})
	fs.Parse(flagPart)

	if *user == *system {
		fatal(fmt.Errorf("specify exactly one of --user or --system"))
	}
	if len(positional) != 2 {
		fatal(fmt.Errorf("usage: envctl set <NAME> <VALUE> (--user|--system)"))
	}
	name, value := positional[0], positional[1]
	if strings.TrimSpace(name) == "" {
		fatal(fmt.Errorf("variable name cannot be empty"))
	}

	sc := scopeUser
	if *system {
		sc = scopeSystem
	}

	oldValue, oldType, existed, err := getPermanentValue(sc, name)
	if err != nil {
		fatal(err)
	}

	regType := uint32(regSZ)
	switch strings.ToLower(*typeOverride) {
	case "sz":
		regType = regSZ
	case "expand":
		regType = regExpandSZ
	case "":
		if existed {
			regType = oldType // preserve what's already there
		} else if strings.Contains(value, "%") {
			regType = regExpandSZ // looks like it references another var, e.g. %USERPROFILE%
		}
	default:
		fatal(fmt.Errorf(`--type must be "sz" or "expand", got %q`, *typeOverride))
	}

	fmt.Printf("Scope:        %s\n", sc)
	fmt.Printf("Name:         %s\n", name)
	if existed {
		fmt.Printf("Current value: %s\n", oldValue)
	} else {
		fmt.Printf("Current value: (not set)\n")
	}
	fmt.Printf("New value:    %s\n", value)
	fmt.Printf("Registry type: %s\n", regTypeName(regType))

	if *dryRun {
		fmt.Println("\n(dry run — nothing written)")
		return
	}

	if !*yes {
		fmt.Print("\nWrite this value? [y/N] ")
		reader := bufio.NewReader(os.Stdin)
		line, _ := reader.ReadString('\n')
		line = strings.TrimSpace(strings.ToLower(line))
		if line != "y" && line != "yes" {
			fmt.Println("aborted, nothing written")
			return
		}
	}

	if err := setPermanentValue(sc, name, value, regType); err != nil {
		fatal(err)
	}
	broadcastEnvChange()

	// verify by reading it back
	readBack, _, _, err := getPermanentValue(sc, name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warning: wrote the value but could not verify by reading it back: %v\n", err)
	} else if readBack != value {
		fmt.Fprintf(os.Stderr, "warning: read-back value does not match what was written (got %q)\n", readBack)
	} else {
		fmt.Println("done — verified by reading the value back from the registry.")
	}
	fmt.Println("Note: already-open terminal windows will not see this until you open a new one.")
}

func regTypeName(t uint32) string {
	if t == regExpandSZ {
		return "REG_EXPAND_SZ (expands %OTHER_VARS%)"
	}
	return "REG_SZ (literal string)"
}

// ---------- install ----------

func cmdInstall(args []string) {
	fs := flag.NewFlagSet("install", flag.ExitOnError)
	dirFlag := fs.String("dir", "", `install directory (default: %LOCALAPPDATA%\Programs\envctl)`)
	yes := fs.Bool("yes", false, "skip the confirmation prompt")
	dryRun := fs.Bool("dry-run", false, "show what would happen, change nothing")
	flagPart, _ := splitArgs(args, []string{"dir"})
	fs.Parse(flagPart)

	srcPath, err := os.Executable()
	if err != nil {
		fatal(fmt.Errorf("could not determine my own path: %w", err))
	}

	installDir := *dirFlag
	if installDir == "" {
		appData := os.Getenv("LOCALAPPDATA")
		if appData == "" {
			fatal(fmt.Errorf("%%LOCALAPPDATA%% is not set — pass --dir explicitly"))
		}
		installDir = appData + `\Programs\envctl`
	}
	destPath := installDir + `\envctl.exe`
	alreadyAtDest := strings.EqualFold(filepath.Clean(srcPath), filepath.Clean(destPath))

	currentPath, pathType, pathExisted, err := getPermanentValue(scopeUser, "Path")
	if err != nil {
		fatal(err)
	}
	newPath, alreadyInPath := appendToPath(currentPath, installDir)

	if alreadyAtDest {
		fmt.Printf("Already running from: %s (skipping copy)\n", destPath)
	} else {
		fmt.Printf("Copy:         %s\n", srcPath)
		fmt.Printf("           -> %s\n", destPath)
	}
	if alreadyInPath {
		fmt.Printf("USER PATH:    already contains %s — no change needed\n", installDir)
	} else {
		fmt.Printf("USER PATH:    will append %s\n", installDir)
	}
	fmt.Println("Scope:        USER only (not SYSTEM) — no Administrator required")

	if *dryRun {
		fmt.Println("\n(dry run — nothing copied or written)")
		return
	}

	if !*yes {
		fmt.Print("\nProceed? [y/N] ")
		reader := bufio.NewReader(os.Stdin)
		line, _ := reader.ReadString('\n')
		line = strings.TrimSpace(strings.ToLower(line))
		if line != "y" && line != "yes" {
			fmt.Println("aborted, nothing changed")
			return
		}
	}

	if !alreadyAtDest {
		if err := os.MkdirAll(installDir, 0o755); err != nil {
			fatal(fmt.Errorf("creating %s: %w", installDir, err))
		}
		// Deliberately NOT overwriting a currently-executing exe in place —
		// Windows file-locking behavior for that is not something this was
		// able to verify. If you're already running from destPath, this
		// step is skipped entirely (see alreadyAtDest above).
		if err := copyFile(srcPath, destPath); err != nil {
			fatal(fmt.Errorf("copying exe: %w", err))
		}
		fmt.Println("copied.")
	}

	if alreadyInPath {
		fmt.Println("PATH unchanged (already present).")
	} else {
		regType := uint32(regExpandSZ) // Windows' own default for Path
		if pathExisted {
			regType = pathType // preserve whatever type was already there
		}
		if err := setPermanentValue(scopeUser, "Path", newPath, regType); err != nil {
			fatal(fmt.Errorf("writing PATH (the exe WAS copied to %s; add it to PATH yourself or re-run `envctl install`): %w", destPath, err))
		}
		broadcastEnvChange()

		readBack, _, _, err := getPermanentValue(scopeUser, "Path")
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: wrote PATH but could not verify by reading it back: %v\n", err)
		} else if !pathContainsDir(readBack, installDir) {
			fmt.Fprintf(os.Stderr, "warning: read back PATH does not contain %s — something went wrong\n", installDir)
		} else {
			fmt.Println("PATH updated — verified by reading it back from the registry.")
		}
	}

	fmt.Println("\ndone. Open a NEW terminal window, then run: envctl help")
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	tmp := dst + ".tmp"
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := out.ReadFrom(in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	// atomic-ish swap: if dst already exists (reinstall/upgrade), replace it
	return os.Rename(tmp, dst)
}

// ---------- config ----------

func cmdConfig(args []string) {
	if len(args) == 0 || args[0] != "init" {
		fmt.Println(`usage: envctl config init [--config PATH]`)
		return
	}
	fs := flag.NewFlagSet("config init", flag.ExitOnError)
	configPath := fs.String("config", "", "path to write (default: %APPDATA%\\envctl\\config.json)")
	fs.Parse(args[1:])

	path := *configPath
	if path == "" {
		path = defaultConfigPath()
	}
	if _, err := os.Stat(path); err == nil {
		fatal(fmt.Errorf("%s already exists — remove it first or pass --config with a new path", path))
	}
	if err := writeDefaultConfig(path); err != nil {
		fatal(err)
	}
	fmt.Println("wrote default config to", path)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
