# AGENTS.md

This repository already contains `CLAUDE.md` with detailed build, architecture, and workflow guidance.

## Non-obvious constraints

- **Darwin-only:** Every source file has `//go:build darwin`. Builds/tests fail on Linux/Windows regardless of `CGO_ENABLED=1`. The C source (`smc.h`, `smc.c`) and IOKit/HID dependencies are macOS-only.
- **Two Go modules:** Root (`github.com/dkorunic/iSMC`) and nested (`github.com/dkorunic/iSMC/gosmc`). When updating gosmc dependencies, `cd` into that directory first.
- **`smc/sensors.go` is code-generated.** Do not edit it manually — run `task generate` (reads data from `src/{temp,fans,power,voltage,current}.txt`).
- **`task build` has a fixed order:** generate → fmt → build. Breaking this order produces stale sensor data.
