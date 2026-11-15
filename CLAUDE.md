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

`guess` classifier changes are checked by `TestReplayRuns` (`internal/cmd/guess_capture_test.go`), which replays the saved runs in `internal/cmd/testdata/*-run.json` against golden core lines in `*.cores` (`go test ./internal/cmd/ -run TestReplayRuns -update` rewrites them; review the diff). Fanless Macs (`m4-air-run`, M4 MacBook Air Mac16,12): E-phase load warms the whole die by 3–4 °C, so by the end of a phase P and E groups rise alike and the median-normalized end rise ties them. `onsetPhase` breaks such ties from the 1 s load snapshots (P triplets +1.6–1.8 °C at 1 s under P load vs ≤ 0 under E load). Scoring by median probe or by the rise above the die-wide median both failed against the fixtures. Core groups that copy another group's key or are the maximum of keys in other groups (`dropAliasGroups`) are listed as cluster sensors, not cores. Remaining gaps: the Air's four "P groups" (`Tp0U`–`Tp0f`) rise exactly like its P-cores and feed `TCMz`, and the M4 Pro's fabric triplets (`Tp2I`–`Tp2X`) rise only ~7% less than its cores, so both still appear as extra P-cores; whole-tier loads cannot separate them. The base M3 (`m3-air-run`, MacBook Air 15 Mac15,13) is worse: sixteen P triplets (`Tp00`–`Tp10`) rise alike for 4 P-cores, the bank maxima `Tp1L` and `Tp1I` (max of `Tp14`/`Tp18`) still land as P groups, and the E load warms the three `Te*` groups only ~2 °C, so rebalancing adds `Tp16` to the E tier. Per-core attribution is not achievable: Apple Silicon has no thread affinity, a single spinning thread migrates across its cluster (on an M1 Ultra the busiest CPU held it only 31–55% of a 3 s window), and correlating 1 s temperature changes with per-CPU busy share gave no core structure (r 0.1–0.6, the same CPU topping keys on both dies). Don't retry a `--per-core` mode without a pinning mechanism. The SMC keeps its own sensor index, `ftP*`/`ftE*`/`ftG*`/`ftS*` (`guess_ftindex.go`; TG Pro reads it too): each entry mirrors probe B of one `T*` group (key order on M1 Ultra and base M3, not on M3 Pro: `ftP0=Tp0V`, `ftP1=Tp05`, …, read as six pairs plus two probes but not verifiable from 1 s snapshots), exactly at idle and lagging up to ~5 °C under load changes. `guess` samples it, resolves each entry to its nearest per-sensor key (lowercase second letter, so not `TVD0`/`TCMb`), strips it, and lists `Tp`/`Te` core groups with no indexed key as cluster sensors; with `ftE` present on M3+ it does not fill the E tier with P groups. It marks which groups the firmware controls by, not tiers or cores: on M1 `ftP` covers all 20 M1 Ultra core triplets (E-cores included) and `ftE` the die keys; on base M3 `ftP` adds the `Tp13`/`Tp17` probes. M3 Pro uses generic Group labels for the same reason as base M3: twelve indexed triplets for 6 P-cores, plus the GPU-warmed probes `Tp0u-w`/`Tp0y-10` (formerly "Core 9") and bank maxima `Tp3P`/`Tp3X`; M3 Max/Ultra carry the same P rows (the loaded `report-m3-max.txt` matches key for key; its 48 gated P keys stay unmapped). M3 Pro E is three indexed Groups (`Te04-06`, `Te0G-I`, `Te0R/S/U/T`) plus probe `Te0P`: `Te0U` = `Te0S` + 0.500 in every Pro dump, so it is not a core; M3 Max keeps its E-core labels. Base M3 `Te0O`/`Te0P` are unindexed E probes. A constant offset between two keys of different suffixes (not only probe A/B) means one is derived from the other: check `Te0U`-style pairs before counting a key as a core. Only `report-m3.txt`, `report-m3-pro-5.txt` and `m1-ultra-ft-run` carry the index so far; older dumps hold `T*` keys only. TG Pro (`/Applications/TG Pro.app`, key names obfuscated as one-character `NSMutableString` appenders) maps cores from a per-model table of one key per core, mostly probe B or every Nth `ft` entry, not from measurement; treat it as a hint and check it against dumps and runs. Checked: M1 Ultra and the M2 `Tp0b` aggregate agree with us; its M2 Pro/Max E-cores (the 4.633 B − A offset class) read P level in the stats#1286 Cinebench and all-core dumps (see the M2 comment in `src/temp.txt`), its M6 P-cores `Tp07`/`Tp09` rise like Super cores in `m6-run`, and its base M4 and base M3 E lists count copies (`Te0X` = `Te0T`) or two keys of one triplet (`Te05`/`Te06`). Open leads: it reads M5 Pro/Max P-cores from `Tm*` (labelled Memory here; 17 of them read one identical value in the idle `m5-pro-2`/`m5-max-2` dumps, like a parked cluster) and M5 E-cores from `sd4*` keys. The MacBook Air 13 M5 (`report-m5-2.txt`, `m5-air-run`) confirms base M5 `Tp00-0G` as Super cores and `Te04/08/0C/0R` as E-cores by load (only four for six E-cores); `Tp0O-0a` ("CPU Probe 1-4") are second Super-level probes, warmer under GPU load, and `Tp0p`-`Tp1E` ("CPU Probe 5-10") follow neither tier; its 47 `sd*` keys are firmware die estimates (~6 °C above the probes, several byte-equal to `TVDA`/`TVD0`/`TVDP`/`TVDG`), grouped 4 (`sd0a-0u`) and 6 (`sd43-4M`) like the cores. `guess` now samples `sd*` (never as core groups), so the next M5 run shows which tier each follows; the `Tm*` lead still needs an M5 Pro/Max run. `guess` also prints aliases and maxima found by exact equality in ≥ 90% of the raw load snapshots (`guess_aggregates.go`); treat long maximum member lists as hints, since quantized readings can coincide. M4 Pro/Max report P-cores through two alternating key banks (`Tp00`… and `Tp1i`…), the idle one parked at exactly 40.0 °C (`smc.GatedTempCelsius`): `keepReading` stops a parked read from overwriting a live one in the display, and the `guess` sampler drops 40.0 readings.

Note: `smc` (public) imports `internal/platform`. Go allows this because the import sits within the same module, but anything `smc` re-exports from `platform` effectively becomes part of `smc`'s public API surface — keep that interaction narrow.

### Hardware Abstraction

The tool supports two distinct hardware paths that are merged at the output layer:

- **`gosmc/`** — nested Go module (`github.com/dkorunic/iSMC/gosmc`). CGo bindings to Apple IOKit's SMC interface via `smc.h`. Handles Intel/PPC Macs. Contains IOKit constants (`values.go`) and wrapper functions for `SMCOpen`/`SMCClose`/`SMCReadKey`/`SMCCall`/`SMCWriteKey`.

- **`smc/`** — uses `gosmc` to query named SMC keys. `sensors.go` is **code-generated** (do not edit directly) — regenerate with `task generate` or `go generate ./smc`. The generator reads colon-delimited data from `../src/{temp,fans,power,voltage,current}.txt`. Supports wildcard keys (`%`) expanded to indices 0–9. A row's platform tag is `All`, `Apple`, a family (`M4`, every M4 SKU), a SKU spelled exactly as `Product.CPU` in `internal/platform/mapping.go` (`M4 Pro`), or `<family> Base` for the base SKU only (`M4 Base`) — see `platformMatches` in `smc/smc.go`. `smc/replay_test.go` (`Test_DumpReplay`) replays every `internal/reports` dump through this mapping; known gaps live in its `replayAllow` list, which fails when an entry goes stale. Base chips use a different key layout from Pro/Max/Ultra, which share one: base M2 and base M3 proved it. When a family row's key means something else on another SKU, copy the row once per SKU (`M3 Pro`, `M3 Max`, `M3 Ultra`; a row takes one tag) and give the base SKU its own `<family> Base` rows. Don't split rows without a dump showing a conflict: a row whose key a SKU lacks prints nothing, and `Test_DumpLiveRows` fails SKU copies that no dump of that SKU reads live (Ultra copies pass because no Ultra dump exists). Base M3 rows use generic Group labels because its run cannot tie any triplet to a core.

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
