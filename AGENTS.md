# thock

A typing test for the terminal, with Monkeytype's scoring. Go TUI. Not a JS/TS web app — do not apply `next16-app`.

## Commands

```sh
go build -o thock .
go test ./...
go vet ./...
gofmt -l .
```

Requires Go 1.25+. Module: `github.com/dawsonxiong/thock`.

Portfolio screenshots: `cd scripts/screenshots && pnpm install && pnpm capture` (see its README).

```sh
thock                     # 30s test, 1k word list
thock --time 60
thock --words 50
thock --mode quotes
thock stats
```

## Stack

- Charm Bubble Tea v2 (`charm.land/bubbletea/v2`), Lipgloss v2, cobra, fang.
- Layout: thin `main.go`, logic in `internal/`.
- Follow the `go-cli` house skill.

## Hard rules

- Pure core, IO at the edges. Pass elapsed time in; do not call `time.Now()` inside domain logic.
- Terminal text uses `[]rune` and display-width, never `len()` on a string.
- Filesystem tests must `t.Setenv` `XDG_*` to `t.TempDir()`.
- No GitHub MCP. No project MCP.
