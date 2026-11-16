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

`guess` classifier changes are checked by `TestReplayRuns` (`internal/cmd/guess_capture_test.go`), which replays the saved runs in `internal/cmd/testdata/*-run.json` against golden core lines in `*.cores` (`go test ./internal/cmd/ -run TestReplayRuns -update` rewrites them; review the diff). Fanless Macs (`m4-air-run`, M4 MacBook Air Mac16,12): E-phase load warms the whole die by 3–4 °C, so by the end of a phase P and E groups rise alike and the median-normalized end rise ties them. `onsetPhase` breaks such ties from the 1 s load snapshots (P triplets +1.6–1.8 °C at 1 s under P load vs ≤ 0 under E load). Scoring by median probe or by the rise above the die-wide median both failed against the fixtures. Core groups that copy another group's key or are the maximum of keys in other groups (`dropAliasGroups`) are listed as cluster sensors, not cores. Remaining gaps: the Air's four "P groups" (`Tp0U`–`Tp0f`) rise exactly like its P-cores and feed `TCMz`, and the M4 Pro's fabric triplets (`Tp2I`–`Tp2X`) rise only ~7% less than its cores, so both still appear as extra P-cores; whole-tier loads cannot separate them. The base M3 (`m3-air-run`, MacBook Air 15 Mac15,13) is worse: sixteen P triplets (`Tp00`–`Tp10`) rise alike for 4 P-cores, the bank maxima `Tp1L` and `Tp1I` (max of `Tp14`/`Tp18`) still land as P groups, and the E load warms the three `Te*` groups only ~2 °C, so rebalancing adds `Tp16` to the E tier. Per-core attribution is not achievable: Apple Silicon has no thread affinity, a single spinning thread migrates across its cluster (on an M1 Ultra the busiest CPU held it only 31–55% of a 3 s window), and correlating 1 s temperature changes with per-CPU busy share gave no core structure (r 0.1–0.6, the same CPU topping keys on both dies). Don't retry a `--per-core` mode without a pinning mechanism. The SMC keeps its own sensor index, `ftP*`/`ftE*`/`ftG*`/`ftS*` (`guess_ftindex.go`; TG Pro reads it too): each entry mirrors probe B of one `T*` group (key order on M1 Ultra and base M3, not on M3 Pro: `ftP0=Tp0V`, `ftP1=Tp05`, …, read as six pairs plus two probes but not verifiable from 1 s snapshots), exactly at idle and lagging up to ~5 °C under load changes. `guess` samples it, resolves each entry to the per-sensor key (lowercase second letter, so not `TVD0`/`TCMb`) it equals exactly in the most snapshots (≥ 2), else the nearest mean (an uppercase index such as `ftPN`, a copy of the M4 Max SSD key `TH0x`, or `ftDO` is not a tier entry), strips it, and lists `Tp`/`Te` core groups with no indexed key as cluster sensors; with `ftE` present on M3+ it does not fill the E tier with P groups. It marks which groups the firmware controls by, not tiers or cores: on M1 `ftP` covers all 20 M1 Ultra core triplets (E-cores included) and `ftE` the die keys; on base M3 `ftP` adds the `Tp13`/`Tp17` probes. M3 Pro uses generic Group labels for the same reason as base M3: twelve indexed triplets for 6 P-cores, plus the GPU-warmed probes `Tp0u-w`/`Tp0y-10` (formerly "Core 9") and bank maxima `Tp3P`/`Tp3X`; M3 Max/Ultra carry the same P rows (the loaded `report-m3-max.txt` matches key for key; its 48 gated P keys stay unmapped). M3 Pro E is three indexed Groups (`Te04-06`, `Te0G-I`, `Te0R/S/U/T`) plus probe `Te0P`: `Te0U` = `Te0S` + 0.500 in every Pro dump, so it is not a core; M3 Max keeps its E-core labels. Base M3 `Te0O`/`Te0P` are unindexed E probes. A constant offset between two keys of different suffixes (not only probe A/B) means one is derived from the other: check `Te0U`-style pairs before counting a key as a core. Full dumps with the index: `report-m3.txt`, `report-m3-pro-5.txt`, `report-m3-max-2.txt`, `report-m4-max.txt`, `report-m4-pro-4/5.txt`, `report-m4-8/9.txt`, `report-m2-pro-2.txt`, `report-m1-pro-4.txt`, `report-m1-max-2.txt`, `report-a18.txt` (and `report-m5-2.txt`, `report-m5-pro-3.txt`, `report-m5-max-2.txt` with `sd*`), plus `m1-ultra-ft-run`. The M1 Pro/Max index matches no key exactly in a single dump (it lags its sources), and `report-m4-8` caught it mid-change; base M4 (`report-m4-9`) indexes P-cores 1-4 and P Groups 1-4 in pairs, three cluster aggregates and E-cores 1-4 (`Te05/09/0H/0S`), as mapped. Both M2 Pro/Max (`report-m2-pro-2`: Probe Groups live, P-core triplets at sentinels) and M4 Pro (`report-m4-pro-5`, Mac16,7: `Tp1i` bank and `Tpx8`–`TpxD` live) switch to their second P bank, and the index lists both banks; the other dumps hold `T*` keys only. TG Pro (`/Applications/TG Pro.app`, key names obfuscated as one-character `NSMutableString` appenders) maps cores from a per-model table of one key per core, mostly probe B or every Nth `ft` entry, not from measurement; treat it as a hint and check it against dumps and runs. Checked: M1 Ultra and the M2 `Tp0b` aggregate agree with us; its M2 Pro/Max E-cores (the 4.633 B − A offset class) read P level in the stats#1286 Cinebench and all-core dumps (see the M2 comment in `src/temp.txt`), its M6 P-cores `Tp07`/`Tp09` rise like Super cores in `m6-run`, and its base M4 and base M3 E lists count copies (`Te0X` = `Te0T`) or two keys of one triplet (`Te05`/`Te06`). It reads M5 Pro/Max P-cores from `Tm*`, confirmed by `m5-max-run` (Mac17,6): Super-only load heats all 18 `Tp00`–`Tp0y` keys (six Super triplets, labelled `CPU Super Core N Probe M` because the hottest probe moves with load) and leaves `Tm*` idle, and P load lifts `Tm*` ~8 °C; `Tm*` are two 19-probe P clusters (one reads a single value while parked) with no per-core attribution. M5 Max has no CPU ft index; its 153 `sd*` keys copy no `T*` key but split by tier in `m5-max-run`: `sdA9`–`sdAz` (18) follow the Super cores, `sd7q`–`sd8e` and `sd90`–`sd9n` the P cores, and `sd0d`–`sd2P`, `sd4e`–`sd63` and `sd6I`–`sd7h` the GPU (see the M5 Pro/Max comment in `src/temp.txt`), so the old `sd4*` E-core lead is closed. The MacBook Air 13 M5 (`report-m5-2.txt`, `m5-air-run`) confirms base M5 `Tp00-0G` as Super cores and `Te04/08/0C/0R` as E-cores by load (only four for six E-cores); `Tp0O-0a` ("CPU Probe 1-4") are second Super-level probes, warmer under GPU load, and `Tp0p`-`Tp1E` ("CPU Probe 5-10") follow neither tier; its 47 `sd*` keys are each one `T*` key plus a fixed per-unit offset (σ 0 over the 30 snapshots of `m5-mbp14-run`, MacBook Pro 14 Mac17,2, full dump `report-m5.txt`; `sd37-3l` = the Super probes, `sd43-4M` = `Tp0p-1E`, `sd0a/0d/0g/0u` = `Te*`, `sd16-33` = `Tg*`), so they stay unmapped; that run repeats the Air's classification; the M5 Pro relabel rests on the shared layout and the loaded `report-m5-pro.txt` (triplet 6 cool on a 5-Super SKU), not on a run. A key family with no live baseline key (M4 Max: both banks and `ftP*` parked at 40.0; M5 Max: every core key idles below `guessTempMin`) is scored against that floor in `deltaTemps`; `resolveFtIndex` takes keys from all snapshots. M3 Max has a second 48-key `Tp` bank, gated in every dump, that goes live ~2 s into P load (`m3-max-run`); it is unmapped. The index covers it: `ftP0`–`ftPd` are the M3 Pro bank-A list byte for byte and `ftPe`–`ftPr` copy bank B, but under load the nearest mean picked bank-B probes for bank-A entries, hence the exact-match rule. With `ftE` present, `assignTeByPrefix` runs before `trimToLayout` (the full 12-group P tier would otherwise cut the indexed `Te` groups); without it the trim goes first, since M6 `Te0c`–`Te0i` are die probes. `guess` also prints aliases and maxima found by exact equality in ≥ 90% of the raw load snapshots (`guess_aggregates.go`); treat long maximum member lists as hints, since quantized readings can coincide. M4 Pro/Max report P-cores through two alternating key banks (`Tp00`… and `Tp1i`…; on the 12-P-core M4 Max Fabric 1–2 of each bank are P-cores 11–12, `m4-max-run`), the idle one parked at exactly 40.0 °C (`smc.GatedTempCelsius`): `keepReading` stops a parked read from overwriting a live one in the display, and the `guess` sampler drops 40.0 readings.

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
