# envctl

[![CI](https://github.com/cumulus13/envctl/actions/workflows/ci.yml/badge.svg)](https://github.com/cumulus13/envctl/actions/workflows/ci.yml)
[![Release](https://github.com/cumulus13/envctl/actions/workflows/release.yml/badge.svg)](https://github.com/cumulus13/envctl/actions/workflows/release.yml)

a Windows CLI for listing, searching, and
permanently setting environment variables — colored (hex, not RGB),
emoji-tagged, borderless table output.

## What makes this safe (read this)

This is a **completely different technique** from the risky PEB/process-memory
approach discussed earlier in this conversation. `envctl` only ever does two
things to the OS:

1. **Reads/writes the registry** — `HKCU\Environment` (USER scope) and
   `HKLM\SYSTEM\CurrentControlSet\Control\Session Manager\Environment`
   (SYSTEM scope). These are the exact same keys `regedit`, the System
   Properties → Environment Variables dialog, and `setx.exe` use. There is
   nothing undocumented or version-fragile about this — it's the public,
   stable Win32 registry API (`RegOpenKeyExW`/`RegQueryValueExW`/
   `RegSetValueExW`), unchanged since Windows NT.
2. **Broadcasts `WM_SETTINGCHANGE`** after a write, the same notification
   `setx` sends, so new processes/shells pick up the change. It does not
   and cannot reach into another process's memory.

`set` **only ever touches the one named value you give it** — it reads that
single value, writes that single value back. It never reads, rewrites, or
replaces the whole Environment block, so there is no way for it to wipe out
`PATH` or anything else sitting next to it.

### Hard limitation (not a bug, a Windows fact)

A registry write does **not** change the environment of terminal windows
that are already open — only of processes started *after* the write (new
`cmd`/PowerShell windows, or anything that re-reads
`HKCU`/`HKLM`\...\`Environment` on launch). This is true of `setx`, the GUI
dialog, and every other tool — there is no way around it without the
risky memory-injection technique, which this tool deliberately does not use.

## Install

No PowerShell, no shell script, no second language touching your registry
at all. The binary installs itself, using the exact same tested registry
code as `set` — PATH is just a normal value named `Path` under the same
`Environment` key, so there was no reason to write it twice.

### Option A — the binary installs itself (recommended, no admin needed)

```
dist\envctl.exe install
```

(or wherever you extracted `envctl.exe` from the release zip — run it
from there). It:

1. Shows you exactly what it's about to do (copy path, PATH change) and
   asks `[y/N]` first — same confirmation flow as `set`.
2. Copies itself to `%LOCALAPPDATA%\Programs\envctl\envctl.exe`.
3. Appends that folder to your **USER** PATH only — never SYSTEM, never
   needs Administrator — and only if it isn't already there.
4. Reads PATH back from the registry afterward to confirm the write
   actually took, instead of assuming it worked.

```
dist\envctl.exe install --dry-run          # shows what would happen, changes nothing
dist\envctl.exe install                    # asks [y/N] before touching PATH
dist\envctl.exe install --yes              # skips the prompt
dist\envctl.exe install --dir D:\tools\envctl
```

Open a new terminal afterward — same Windows limitation as `set`: already-open
windows won't see the PATH change.

### Option B — download the release zip manually, no install at all

Grab the zip for your architecture from the
[Releases page](https://github.com/cumulus13/envctl/releases)
(windows-amd64, windows-386, or windows-arm64) and just run `envctl.exe`
from wherever you extracted it — `install` is a convenience, not a
requirement.

### Option C — build from source with Go

```
go install github.com/cumulus13/envctl@latest
```

## Build from source

Requires only the Go toolchain — zero external dependencies (stdlib +
direct `advapi32.dll`/`user32.dll`/`kernel32.dll` calls via `syscall`).

```powershell
git clone https://github.com/cumulus13/envctl.git
cd envctl
go build -o envctl.exe .
```

Or cross-compile from Linux/macOS:
```bash
GOOS=windows GOARCH=amd64 go build -o envctl.exe .
```

This was cross-compiled and `go vet`-checked for windows/amd64, windows/386,
and windows/arm64 as part of producing it, and `.github/workflows/ci.yml`
repeats those same checks (fmt, vet, `go test`, cross-compile matrix) on
every push/PR. I could not run it against a real Windows registry — that
part you'll need to verify yourself; **start with `--dry-run` and
non-critical variable names.**

## CI/CD

- **`.github/workflows/ci.yml`** — on every push/PR: `gofmt -l` check,
  `go vet` (windows/amd64), `go test ./...` (the platform-neutral logic:
  `args_test.go` plus anything else buildable outside `_windows.go` files),
  then a cross-compile matrix building windows/amd64, windows/386, and
  windows/arm64 as uploaded artifacts.
- **`.github/workflows/release.yml`** — on pushing a tag matching `v*.*.*`:
  builds all three arches with the version stamped in via
  `-ldflags -X main.version=...`, zips each with the README, and publishes
  a GitHub Release with the zips attached (`envctl version` prints the
  stamped tag instead of `dev`).

## Usage

```
envctl list   [--current | --user | --system] [--no-color] [--no-emoji] [--config PATH]
envctl find   <pattern> [--regex] [--case-sensitive] [--keys-only] [--values-only]
                        [--current | --user | --system] [--no-color] [--no-emoji]
envctl set    <NAME> <VALUE> (--user | --system) [--type sz|expand] [--yes] [--dry-run]
envctl config init [--config PATH]
```

- No scope flag on `list`/`find` → shows **permanent** vars from both USER
  and SYSTEM registry keys, tagged by source.
- `--current` → shows the **live** environment of this process (i.e. exactly
  what the current terminal would hand to any child process right now).
- `--user` / `--system` → restrict to one permanent scope.

### Examples

```powershell
envctl list
envctl list --current
envctl list --system --no-color
envctl find "JAVA*"
envctl find "^GO" --regex --keys-only
envctl set MY_TOOL_HOME "C:\tools\mytool" --user
envctl set JAVA_HOME "C:\jdk-21" --system --dry-run
envctl set JAVA_HOME "C:\jdk-21" --system --yes
```

`set` always shows the current value, the new value, and the registry type
it will use, and asks `[y/N]` before writing (skip with `--yes`). After
writing it reads the value back from the registry and confirms it matches,
rather than assuming the write worked.

## Config file (colors & icons)

Default location: `%APPDATA%\envctl\config.json`. Generate it with:

```powershell
envctl config init
```

```json
{
  "colors": {
    "key": "#00FFFF",
    "value": "#FFFF00",
    "source_user": "#00FF88",
    "source_system": "#FF8800",
    "source_current": "#AAAAAA",
    "match_highlight": "#FF00FF",
    "dim": "#666666",
    "warning": "#FFAA00",
    "error": "#FF3333"
  },
  "icons": {
    "key": "🔑",
    "user": "👤",
    "system": "🖥️",
    "current": "⚡",
    "match": "🔍",
    "warning": "⚠️",
    "error": "❌",
    "ok": "✅"
  }
}
```

Colors are hex (`#RRGGBB`), converted to 24-bit ANSI truecolor at runtime —
not RGB tuples, as requested. Edit any subset of fields; anything you omit
falls back to the built-in default. `--no-color` / `--no-emoji` or setting
`"no_color": true` / `"no_emoji": true` in the config disable them.

`args.go` exists because the stdlib `flag` package stops parsing at the
first positional argument — without it, `envctl find debug --current`
would silently treat `--current` as a second pattern instead of a flag.
Covered by `args_test.go`.

`path_util.go` holds the PATH-list logic (`install` uses it) as pure,
platform-neutral string functions so it's directly unit-testable without
touching a real registry — covered by `path_util_test.go`.

---

### License

[MIT](LICENSE)


## 👤 Author
        
[Hadi Cahyadi](mailto:cumulus13@gmail.com)
    

[![Buy Me a Coffee](https://www.buymeacoffee.com/assets/img/custom_images/orange_img.png)](https://www.buymeacoffee.com/cumulus13)

[![Donate via Ko-fi](https://ko-fi.com/img/githubbutton_sm.svg)](https://ko-fi.com/cumulus13)
 
[Support me on Patreon](https://www.patreon.com/cumulus13)