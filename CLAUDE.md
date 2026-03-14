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
CGO_ENABLED=1 go test ./output/...
```

## Architecture

### Hardware Abstraction

The tool supports two distinct hardware paths that are merged at the output layer:

- **`gosmc/`** — nested Go module (`github.com/dkorunic/iSMC/gosmc`). CGo bindings to Apple IOKit's SMC interface via `smc.h`. Handles Intel/PPC Macs. Contains IOKit constants (`values.go`) and wrapper functions for `SMCOpen`/`SMCClose`/`SMCReadKey`/`SMCCall`/`SMCWriteKey`.

- **`smc/`** — uses `gosmc` to query named SMC keys. `sensors.go` is **code-generated** (do not edit directly) — regenerate with `task generate` or `go generate ./smc`. The generator reads colon-delimited data from `../src/{temp,fans,power,voltage,current}.txt`. Supports wildcard keys (`%`) expanded to indices 0–9.

- **`hid/`** — Apple Silicon (M-series) sensor support via the HID sensor hub. Uses embedded C (`get.go` has CGo). Reads temperature, current, voltage from IOKit HID services.

### Data Flow

```
cmd/ (Cobra subcommands)
  → output.Factory(OutputFlag)  [selects output format]
    → output.Output interface methods (All, Temperature, Fans, etc.)
      → merge(smc.Get*(), hid.Get*())
        → gosmc.SMCOpen/ReadKey  (Intel)
        → CGo HID calls          (Apple Silicon)
```

All sensor data is returned as `map[string]any` where each entry contains `"key"`, `"value"`, and `"type"` fields.

### Output System

`output/` implements the `Output` interface with four backends selected via `-o` flag:
- `table` — pretty table (default, uses `go-pretty`)
- `ascii` — plain ASCII table
- `json` — JSON
- `influx` — InfluxDB line protocol

`output/outputfactory.go` is the factory. The `GetAll`, `GetTemperature`, etc. vars in `output/output.go` are function variables to enable monkey-patching in tests.

### Module Structure

This repo contains **two Go modules**:
- Root: `github.com/dkorunic/iSMC` (`go.mod` at root)
- Nested: `github.com/dkorunic/iSMC/gosmc` (`gosmc/go.mod`)

When updating dependencies in `gosmc/`, cd into that directory first.

## Code Style & Linting

All files have a `//go:build darwin` constraint — this is macOS-only code. The linter config is in `.golangci.yml` (golangci-lint v2, `default: all` with specific linters disabled). Formatters used: `gci`, `gofmt`, `gofumpt`, `goimports`. Run `task fmt` before committing.
