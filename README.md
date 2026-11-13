# iSMC

[![GitHub license](https://img.shields.io/github/license/dkorunic/iSMC.svg)](https://github.com/dkorunic/iSMC/blob/master/LICENSE)
[![GitHub release](https://img.shields.io/github/release/dkorunic/iSMC.svg)](https://github.com/dkorunic/iSMC/releases/latest)

## About

`iSMC` is a macOS command-line tool for querying the Apple System Management Controller (SMC). It reads a broad set of well-known SMC keys, determines their type and value, and classifies the results into temperature, power, current, voltage, fan, and battery readings. Each key is accompanied by a human-readable description.

In addition to standard SMC support for Intel Mac hardware, `iSMC` supports Apple Silicon: the full M1–M5 line (including Pro/Max/Ultra variants where they exist) plus the A18 Pro used by the MacBook Neo (`Mac17,5`). On these models, temperature, voltage, and current sensors are read through a HID sensor hub; power readings still come through the SMC path.

![Demo](demo.gif)

## Installation

`iSMC` runs on macOS only.

### Homebrew

You can install iSMC using [Homebrew](https://brew.sh/) by adding the `homebrew-tap` tap and installing the `ismc` package:

```shell
brew tap dkorunic/tap
brew install ismc
```

### Manual

Download the appropriate iSMC binary for your platform from [the releases page](https://github.com/dkorunic/iSMC/releases/latest) and install it manually.

### Using go install

You can also install iSMC using the `go install` command:

```shell
CGO_ENABLED=1 go install github.com/dkorunic/iSMC@latest
```

## Usage

```shell
Apple SMC CLI tool that can decode and display temperature, fans, battery, power, voltage and current
information for various hardware in your Apple Mac hardware.

Usage:
  iSMC [flags]
  iSMC [command]

Available Commands:
  all         Display all known sensors, fans and battery status
  batt        Display battery status
  completion  Generate the autocompletion script for the specified shell
  curr        Display current sensors
  fans        Display fans status
  guess       Map SMC temperature sensors to CPU cores by thermal correlation
  help        Help about any command
  hw          Display hardware information
  power       Display power sensors
  raw         Display all raw SMC keys and their byte values
  temp        Display temperature sensors
  version     Print the version number of iSMC
  volt        Display voltage sensors

Flags:
  -h, --help            help for iSMC
  -o, --output string   Output format (ascii, table, json, influx) (default "table")
  -w, --watch duration  Re-read sensors every interval (e.g. 5s); the table view refreshes in place (default 2s, 0 turns it off), other outputs re-print

Use "iSMC [command] --help" for more information about a command.
```

Each command also accepts short and long aliases: `bat`/`batt`/`battery`, `cur`/`curr`/`current`, `fan`/`fans`, `hw`/`hardware`/`info`, `pow`/`power`, `tmp`/`temp`/`temperature`, `vol`/`volt`/`voltage`, `all`/`everything`/`*`.

### Output formats

| Format   | Description                                                     |
| -------- | --------------------------------------------------------------- |
| `table`  | Coloured terminal table, scrollable when it overflows (default) |
| `ascii`  | Plain ASCII table                                               |
| `json`   | JSON                                                            |
| `influx` | InfluxDB line protocol                                          |

When `table` output is longer than the terminal, it opens a scrollable view that fills the terminal and flows rows into as many side-by-side columns as the terminal width fits: `↑`/`↓` (or `j`/`k`) scroll, `PgUp`/`PgDn` page, `g`/`G` jump to top/bottom, `←`/`→` or `Tab` switch category, `q` quits and leaves the last screen in your scrollback. The view re-reads sensors every 2 seconds: `p` pauses and resumes, `-w <interval>` sets another interval and `-w 0` turns refreshing off. Piped or short output prints plain tables instead; use `-o ascii` or pipe the output for unattended runs.

In a terminal, `table` colours values by how high they are: temperatures turn yellow from 80 °C and red from 95 °C; fan speeds turn yellow from 60% and red from 85% of that fan's maximum. No single temperature is shown as critical, because Apple Silicon hotspot sensors routinely read 96–113 °C under sustained load while macOS considers the machine fine. Instead, the `Thermal Pressure` row (in `temp` and `all`) shows macOS's own verdict, read without root: `Nominal` stays plain, `Moderate` is yellow, `Heavy` red, and `Trapping` or `Sleeping` (macOS is throttling hard or about to sleep the machine) bold bright red. Power, voltage and current have no generic limit and stay uncoloured.

`-w`/`--watch <interval>` keeps reading: in a terminal, `table` opens the scrollable view and refreshes it in place (keeping your tab and scroll position), while `ascii`, `json`, `influx` and piped output print a fresh reading every interval until interrupted. For example, `iSMC -w 2s temp` or `iSMC -o influx -w 10s` as a long-running Telegraf input.

`batt` (and `all`) adds MacBook battery details read from the IORegistry without root: cycle count, charge, design and full-charge capacity, health (full-charge share of design capacity, yellow below Apple's 80% service level), voltage, amperage and charge state. Desktops show none, even though some publish an empty battery entry.

`fans` also shows each fan's mode (`auto`, `manual` or `system`), which tells you whether macOS or a fan-control tool is driving the fans, and the `Ftst` force-test switch on Macs that have it.

## Architecture

`iSMC` reads sensors through two hardware paths and unifies them at the output layer:

- **SMC path** — works on Intel/PPC Macs, and on Apple Silicon for power readings. Built on `gosmc`, a CGo wrapper around Apple's IOKit SMC interface.
- **HID path** — Apple Silicon only. Reads temperature, voltage, and current from the HID sensor hub via CGo into IOKit HID services.

```
internal/cmd/            Cobra subcommands
  └─ internal/output/    Output interface — selects table | ascii | json | influx
       ├─ smc.Get*()     named SMC keys (Intel/PPC + Apple Silicon power)
       │    └─ gosmc      CGo → IOKit SMC
       └─ hid.Get*()     HID sensor hub (Apple Silicon temp / volt / curr)
            └─ C/IOKit    CGo → IOKit HID services
```

### Package layout

| Path | Visibility | Purpose |
| ---- | ---------- | ------- |
| `gosmc/` | public (separate Go module) | Low-level IOKit SMC bindings via CGo |
| `smc/` | public | Named-key SMC sensor reader, built on `gosmc` |
| `hid/` | public | Apple Silicon HID sensor reader (embedded C) |
| `internal/cmd/` | internal | Cobra subcommand wiring |
| `internal/output/` | internal | `Output` interface + four backend formatters |
| `internal/platform/` | internal | CPU family / SKU detection used by `smc` |
| `internal/reports/` | internal | Diagnostic report bundler |
| `internal/stress/` | internal | CPU QoS helpers, per-CPU load accounting and Metal GPU load for the `guess` workload (`--gpu`) |

The repo is two Go modules: the root `github.com/dkorunic/iSMC` and the nested `github.com/dkorunic/iSMC/gosmc` (replaced locally via `go.mod`). `internal/` packages are compiler-enforced private to this module; the three public packages are the intended library surface.

`smc/sensors.go` is **code-generated** from `src/{temp,fans,power,voltage,current}.txt` by `smc/gen-sensors.sh`. Regenerate via `task generate` after editing the `.txt` files — do not edit `sensors.go` by hand.

All source files carry a `//go:build darwin` constraint; the project is macOS-only by construction and will not build or test on other platforms regardless of `CGO_ENABLED`.

## Related work

This tool was inspired by several Apple SMC-related projects:

- **SMCKit** — Apple SMC library and tool in Swift: [github.com/beltex/SMCKit](https://github.com/beltex/SMCKit)
- **libsmc** — SMC API in pure C: [github.com/beltex/libsmc](https://github.com/beltex/libsmc)
- **iStats** — Ruby gem for Mac stats: [github.com/Chris911/iStats](https://github.com/Chris911/iStats)
- **smcFanControl** — Fan control tool in Objective-C, includes `smc-command` for raw SMC key queries: [github.com/hholtmann/smcFanControl](https://github.com/hholtmann/smcFanControl)
- **FakeSMC** — Hackintosh kext: [github.com/RehabMan/OS-X-FakeSMC-kozlek](https://github.com/RehabMan/OS-X-FakeSMC-kozlek)
- **VirtualSMC** — Hackintosh kext: [github.com/acidanthera/VirtualSMC](https://github.com/acidanthera/VirtualSMC)
- **osx-cpu-temp** — CPU temperature display in pure C: [github.com/lavoiesl/osx-cpu-temp](https://github.com/lavoiesl/osx-cpu-temp)
- **applesmc.c** — Linux kernel Apple SMC driver: [github.com/torvalds/linux](https://github.com/torvalds/linux/blob/master/drivers/hwmon/applesmc.c)
- **gosmc** — Low-level Go SMC bindings: [github.com/panotza/gosmc](https://github.com/panotza/gosmc)
- **sensors** — Koan-Sin Tan's M1 IOKit demo: [github.com/freedomtan/sensors](https://github.com/freedomtan/sensors)
- **Stats** — Serhiy Mytrovtsiy's macOS Stats app: [github.com/exelban/stats](https://github.com/exelban/stats)

## Todo

Planned features:

- fetch and decode SMC key descriptions from the SMC itself,
- generate and probe random SMC keys,
- persist discovered SMC keys to a configuration file.

## Bugs, feature requests, etc.

Please open an issue or submit a pull request.
