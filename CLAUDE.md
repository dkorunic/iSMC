# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Development Commands

This project uses [Task](https://taskfile.dev) (`task` CLI) as the build runner. `CGO_ENABLED=1` is required for all builds since the code wraps Apple IOKit via CGo.

```sh
task build        # generate sensors, format, build optimized binary (PGO enabled)
task build-debug  # update deps, format, build with race detector + no optimizations
task lint         # format + golangci-lint
task fmt          # go mod tidy, gci, gofumpt, betteralign
task generate     # regenerate smc/sensors.go from src/*.txt data files
task update       # go get -u && go mod tidy
task check        # gomajor list — check for available major version upgrades
task release      # goreleaser release (requires git tag + GITHUB_TOKEN)
```

To build manually without Task:
```sh
CGO_ENABLED=1 go build -trimpath -pgo=auto -o iSMC
```

To run tests:
```sh
CGO_ENABLED=1 go test ./...
# Run a single test package:
CGO_ENABLED=1 go test ./internal/output/...
```

## Architecture

### Package Layout

Public (importable from outside the module):
- `gosmc/` — separate Go module (`github.com/dkorunic/iSMC/gosmc`), low-level IOKit CGo bindings
- `smc/` — named SMC-key sensor reader (Intel/PPC), built on `gosmc`
- `hid/` — Apple Silicon (M-series) HID sensor reader

Internal (compiler-enforced, importable only within this module):
- `internal/cmd/` — Cobra subcommands; also holds the `GitTag`/`GitCommit`/`GitDirty`/`BuildTime` vars set via `-ldflags -X` at build time
- `internal/output/` — `Output` interface + table/ascii/json/influx backends
- `internal/platform/` — CPU family / SKU lookup; consumed by `smc` and `internal/output`
- `internal/reports/` — diagnostic report bundler
- `internal/stress/` — QoS helpers, per-CPU busy share (`ReadCPUTicks`/`BusyShare`, `host_processor_info`) and the Metal GPU load (`StartGPU`, cgo Objective-C) used by `guess` and `guess --gpu`; `guess --save`/`--replay` (`internal/cmd/guess_capture.go`) write and re-analyse a whole run as JSON

`guess` classifier changes are checked by `TestReplayRuns` (`internal/cmd/guess_capture_test.go`), which replays the saved runs in `internal/cmd/testdata/*-run.json` against golden core lines in `*.cores` (`go test ./internal/cmd/ -run TestReplayRuns -update` rewrites them; review the diff). Known gap pinned there: on the fanless M4 MacBook Air (`m4-air-run`, Mac16,12) every P-core triplet lands among the cluster sensors and the Performance phase reports no cores. Without a fan, E-phase load warms the whole die by 3–4 °C, so dividing each phase by its median rise scores every group ~1.0 in both phases: P triplets separate only in raw terms (probes A/B +6.6 °C under P load vs +3.9 °C under E load), and E triplets not at all (+3.5 vs +3.2 °C). Two fixes failed against the replay fixtures — scoring a triplet by its median probe (made E-cores land in P), and scoring the rise above the phase's die-wide median (broke M6 and M1 Ultra) — so this needs a different scoring design. Expect the same on other fanless Airs; map their P-cores from the raw per-phase rises, not from the `guess` verdict.

Note: `smc` (public) imports `internal/platform`. Go allows this because the import sits within the same module, but anything `smc` re-exports from `platform` effectively becomes part of `smc`'s public API surface — keep that interaction narrow.

### Hardware Abstraction

The tool supports two distinct hardware paths that are merged at the output layer:

- **`gosmc/`** — nested Go module (`github.com/dkorunic/iSMC/gosmc`). CGo bindings to Apple IOKit's SMC interface via `smc.h`. Handles Intel/PPC Macs. Contains IOKit constants (`values.go`) and wrapper functions for `SMCOpen`/`SMCClose`/`SMCReadKey`/`SMCCall`/`SMCWriteKey`.

- **`smc/`** — uses `gosmc` to query named SMC keys. `sensors.go` is **code-generated** (do not edit directly) — regenerate with `task generate` or `go generate ./smc`. The generator reads colon-delimited data from `../src/{temp,fans,power,voltage,current}.txt`. Supports wildcard keys (`%`) expanded to indices 0–9. A row's platform tag is `All`, `Apple`, a family (`M4`, every M4 SKU), a SKU spelled exactly as `Product.CPU` in `internal/platform/mapping.go` (`M4 Pro`), or `<family> Base` for the base SKU only (`M4 Base`) — see `platformMatches` in `smc/smc.go`. `smc/replay_test.go` (`Test_DumpReplay`) replays every `internal/reports` dump through this mapping; known gaps live in its `replayAllow` list, which fails when an entry goes stale.

- **`hid/`** — Apple Silicon (M-series) sensor support via the HID sensor hub. Uses embedded C (`get.go` has CGo). Reads temperature, current, voltage from IOKit HID services.

### Data Flow

```
internal/cmd/ (Cobra subcommands)
  → internal/output.Factory(OutputFlag)  [selects output format]
    → internal/output.Output interface methods (All, Temperature, Fans, etc.)
      → merge(smc.Get*(), hid.Get*())
        → gosmc.SMCOpen/ReadKey  (Intel)
        → CGo HID calls          (Apple Silicon)
```

All sensor data is returned as `map[string]any` where each entry contains `"key"`, `"value"`, and `"type"` fields.

### Output System

`internal/output/` implements the `Output` interface with four backends selected via `-o` flag:
- `table` — default. `tableoutput.go` prints lipgloss tables when piped or when they fit the terminal; otherwise `tuioutput.go` opens a scrollable Bubble Tea view: one tab per category, rows flowing newspaper-style into as many side-by-side columns as the width fits (each page is one lipgloss table, one cell per row group). The view re-reads sensors on a `tea.Tick` every 2 s (`tuiRefresh`), or at the `--watch` interval (`p` pauses; an explicit `-w 0` becomes `output.NoRefresh` in `rootCmd.PersistentPreRun` and turns it off); with `--watch`, `output.Run` also opens the view even when the output fits (the read runs in the tick goroutine; ~0.5 s for `all`); other outputs re-print in a loop. Charm libs are v2 under `charm.land/…`, not `github.com/charmbracelet/…`.
- `ascii` — same `TableOutput` with `isASCII`: ASCII borders, no colour, never interactive
- `json` — JSON
- `influx` — InfluxDB line protocol

`internal/output/outputfactory.go` is the factory. The `GetAll`, `GetTemperature`, etc. vars in `internal/output/output.go` are function variables to enable monkey-patching in tests.

### Module Structure

This repo contains **two Go modules**:
- Root: `github.com/dkorunic/iSMC` (`go.mod` at root)
- Nested: `github.com/dkorunic/iSMC/gosmc` (`gosmc/go.mod`)

When updating dependencies in `gosmc/`, cd into that directory first.

Version-injection `-ldflags -X` paths in `Taskfile.yml` and `.goreleaser.yml` target `github.com/dkorunic/iSMC/internal/cmd.{GitTag,GitCommit,GitDirty,BuildTime}` — if `internal/cmd` is ever renamed or moved, those flags must be updated in lockstep (a path mismatch is silent: the binary builds but reports empty version strings).

## Code Style & Linting

All files have a `//go:build darwin` constraint — this is macOS-only code. The linter config is in `.golangci.yml` (golangci-lint v2, `default: all` with specific linters disabled). Formatters used: `gci`, `gofmt`, `gofumpt`, `goimports`. Run `task fmt` before committing.
