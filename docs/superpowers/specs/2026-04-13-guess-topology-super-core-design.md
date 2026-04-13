# guess: CPU topology awareness + Super core detection

**Date:** 2026-04-13
**Status:** Approved
**Files:** `platform/get.go`, `stress/stress_darwin.go`, `cmd/guess.go`, `cmd/guess_test.go`

---

## Problem

The current `guess` command has two deficiencies:

1. **Wrong core numbering for triplet chips (M1, M3, M4).**
   `printMapping` increments `coreIdx` for every sensor. Chips where 3 SMC keys measure
   the same physical core (Tp00 + Tp01 + Tp02 → Core 1) produce 3× too many core labels.
   `seriesKey` correctly groups all-decimal-digit keys (Tp00, Tp01, Tp02 → `Tp**`), but
   `coreIdx` still counts sensors individually within the group instead of counting sub-groups.

2. **No Super core support (M5+).**
   M5 introduces a third performance tier (Super / Performance / Efficiency). The current
   two-phase stress (QoSUserInitiated + QoSBackground) cannot distinguish Super cores from
   regular Performance cores; both heat up during the P-bias phase.

---

## Goal

- Fix triplet grouping using stride/gap detection within each SMC key series.
- Add a third stress phase (`QoSUserInteractive`) on chips with `hw.nperflevels == 3`.
- Expose a `platform.GetPerfLevels()` API so `guess` can read the live CPU topology
  (number of tiers and physical core count per tier) without a static lookup table.
- Keep 2-level behaviour (M1–M4) identical in timing and output format; only the
  core numbering improves.

---

## Architecture

```
runGuess
  → platform.GetPerfLevels()           // sysctl: hw.nperflevels + hw.perflevelN.*
  → determine phases []phaseSpec       // 2 phases (M1–M4) or 3 phases (M5+)
  → avgRawTemps() → baseline
  → for each phaseSpec: runAllCoresPhase(qosClass, baseline) → map[string]float32
  → printMapping(family, numCPU, perfLevels, []phaseResult)
      → classifySensors()              // dominant-phase + cluster-ratio logic, N-phase aware
      → groupBySeries()                // existing: bucket keys by non-numeric pattern
      → groupByStrideWithinSeries()    // NEW: stride/gap split within each bucket
      → emit labelled output
```

---

## Components

### Added: `platform.PerfLevel` + `platform.GetPerfLevels`

```go
// PerfLevel describes one CPU performance tier as reported by the macOS sysctl
// hw.perflevel{N}.* hierarchy.
type PerfLevel struct {
    Name        string // hw.perflevelN.name,  e.g. "Performance" / "Efficiency"
    PhysicalCPU int    // hw.perflevelN.physicalcpu
    LogicalCPU  int    // hw.perflevelN.logicalcpu
}

// GetPerfLevels returns the CPU performance levels for the current machine,
// ordered from highest to lowest performance (perflevel0 first).
// Returns nil if hw.nperflevels is unavailable or zero.
func GetPerfLevels() []PerfLevel
```

Reads via `sysctlbyname`:
- `hw.nperflevels` → N
- `hw.perflevelN.name`, `hw.perflevelN.physicalcpu`, `hw.perflevelN.logicalcpu` for each tier

Falls back gracefully (returns nil) if any sysctl call fails — `runGuess` treats nil as 2-level.

### Added: `stress.QoSUserInteractive`

```go
// QoSUserInteractive maps to QOS_CLASS_USER_INTERACTIVE (0x21).
// On 3-tier chips (M5+) the OS scheduler preferentially routes these threads
// to Super cores (perflevel0) rather than regular Performance cores (perflevel1).
QoSUserInteractive = 0x21
```

### Added in `cmd/guess.go`: `phaseSpec`, `phaseResult`

```go
type phaseSpec struct {
    label   string // "Super", "Performance", "Efficiency"
    qos     int    // QoS class constant from stress package
    cores   int    // physicalCPU for this tier (from GetPerfLevels)
}

type phaseResult struct {
    spec   phaseSpec
    deltas map[string]float32
}
```

### Added in `cmd/guess.go`: `groupByStrideWithinSeries`

```go
// groupByStrideWithinSeries splits a series' sorted sensor list into sub-groups
// based on gaps between consecutive numericValues.
//
// Rules:
//   - If all consecutive differences are equal (uniform stride): each sensor is
//     its own group.  This covers M5-style single-sensor-per-core series:
//     Tp00/04/08 (stride 4) → [[Tp00],[Tp04],[Tp08]] → 3 cores.
//   - Otherwise (non-uniform gaps): split whenever the gap exceeds minDiff.
//     This covers M1/M3 triplets: diffs [1,1,2,1,1,2,...] → minDiff=1,
//     split at every 2 → [[Tp00,Tp01,Tp02],[Tp04,Tp05,Tp06],...] → cores × 3.
//   - A single-element slice is returned as one group.
func groupByStrideWithinSeries(sensors []string) [][]string
```

### Modified: `runGuess`

Replaces the hard-coded 2-phase call sequence with a dynamic loop:

```go
phases := buildPhases(platform.GetPerfLevels())
// buildPhases returns:
//   2-level → [{Performance, QoSUserInitiated, pcpu0}, {Efficiency, QoSBackground, pcpu1}]
//   3-level → [{Super, QoSUserInteractive, pcpu0}, {Performance, QoSUserInitiated, pcpu1},
//               {Efficiency, QoSBackground, pcpu2}]
//   nil/fallback → 2-level defaults with cores=0

for i, phase := range phases {
    // stress + sample → phaseResult
    // inter-phase cooldown (except after last phase)
}
printMapping(family, numCPU, perfLevels, results)
```

### Modified: `printMapping`

New signature:
```go
func printMapping(family string, numCPU int, perfLevels []platform.PerfLevel, results []phaseResult)
```

**Classification** (replaces the current P/E binary split):

```
for each key in union(all phase deltas):
    find dominant = phase with highest delta ≥ guessOutputThreshold
    if none → drop
    find second = phase with second-highest delta ≥ guessOutputThreshold
    if second exists AND min(dominant,second)/max(dominant,second) ≥ (1-guessClusterRatio):
        → cluster/package sensor
    else:
        → assign to dominant phase's type
```

This generalises correctly to both 2-phase and 3-phase without special-casing.

**Core index assignment** uses stride-detected sub-groups:

```go
coreIdx := 1
for _, sk := range sortedSeriesKeys(groups) {
    subGroups := groupByStrideWithinSeries(groups[sk])
    fmt.Printf("// Series %-6s → %d sensor(s) in %d group(s) → %s Core %d..%d\n",
        sk, len(groups[sk]), len(subGroups), label, coreIdx, coreIdx+len(subGroups)-1)
    for _, sg := range subGroups {
        for _, k := range sg {
            fmt.Printf("%s Core %d:%s:%s\n", label, coreIdx, k, family)
        }
        coreIdx++
    }
}
```

**Output header** includes topology summary:

```
// M5 (22 logical CPUs) – guessed sensor mappings
// Topology: 3 perf level(s) | perflevel0 6 phys-CPU "Performance" (→ Super)
//                            | perflevel1 12 phys-CPU "Performance"
//                            | perflevel2 4 phys-CPU "Efficiency"
// WARNING: automated correlation is approximate; always verify on real hardware.
// ────────────────────────────────────────────────────────────────────────────────
```

---

## Stride Detection: Worked Examples

| Series | numericValues | diffs | uniform? | Sub-groups |
|---|---|---|---|---|
| M1 `Tp**` | 0,1,2,4,5,6,8,9,10 | 1,1,**2**,1,1,**2**,1,1 | no | [0,1,2],[4,5,6],[8,9,10] |
| M5 `Tp**` | 0,4,8 | 4,4 | yes | [0],[4],[8] |
| M5 `Tp*C` | 0 | — | yes (1 elem) | [0] |
| M4 `Tp**` | 0,1,2,4,5,6,8,9,21…29 | 1,1,**2**,1,1,**2**,1,**12**,1,… | no | [0,1,2],[4,5,6],[8,9],[21…29] |

M4 note: the large gap (9→21, diff=12) cleanly separates the Tp0x range from Tp2x, but
[21,22,...,29] remains one 9-sensor group because its internal diffs are uniform (all 1).
The output emits a comment noting that manual review is recommended for such groups.

---

## Classification: 2-Phase vs 3-Phase

### 2-level (M1–M4, `hw.nperflevels == 2`)
```
phases: [Performance(QoSUserInitiated), Efficiency(QoSBackground)]
labels: "CPU Performance Core N" / "CPU Efficiency Core N"
cluster: both phases within guessClusterRatio → cluster comment
```
Behaviour identical to current implementation; only the coreIdx assignment changes.

### 3-level (M5+, `hw.nperflevels == 3`)
```
phases: [Super(QoSUserInteractive), Performance(QoSUserInitiated), Efficiency(QoSBackground)]
labels: "CPU Super Core N" / "CPU Performance Core N" / "CPU Efficiency Core N"
cluster: top-two phases within guessClusterRatio → cluster comment
```

---

## Edge Cases

| Situation | Handling |
|---|---|
| `hw.nperflevels` sysctl fails or returns 0 | `GetPerfLevels()` returns nil; `buildPhases` falls back to 2-phase defaults |
| 3-level but Super phase yields no sensors | Warning comment emitted; Super section skipped; P/E sections continue |
| Series with 1 sensor | groupByStrideWithinSeries returns `[[sensor]]`; coreIdx increments once |
| Two sensors with equal numericValue in same series | Tie-broken by lexicographic key order (stable); treated as uniform → individual groups |
| M4 large final group (uniform internal diffs) | Emits a `// NOTE: N-sensor group; check against src/temp.txt` comment |
| `GetFamily()` returns "Unknown" | Existing fallback to "Apple"/"Intel" from GOARCH (unchanged) |

---

## Timing

| Config | Phases | Total |
|---|---|---|
| 2-level (M1–M4) | 2 | ~34s (unchanged) |
| 3-level (M5+) | 3 | ~55s (one extra 8s stress + 12s cooldown + 1.5s sample) |

---

## File Map

| File | Change |
|---|---|
| `platform/get.go` | Add `PerfLevel` struct + `GetPerfLevels()` function |
| `stress/stress_darwin.go` | Add `QoSUserInteractive = 0x21` constant |
| `cmd/guess.go` | Add `phaseSpec`, `phaseResult`, `buildPhases`, `groupByStrideWithinSeries`; refactor `runGuess` + `printMapping` |
| `cmd/guess_test.go` | Add `TestGroupByStrideWithinSeries` |

---

## Testing

**Unit:**
- `TestGroupByStrideWithinSeries`: M1 triplets, M5 uniform stride, M4 irregular, single-sensor, two-sensor uniform

**Live system (M4 Pro, current machine):**
1. Run `./iSMC guess`
2. Compare topology header against `sysctl hw.perflevel*`
3. Count "CPU Performance Core" entries — should be ≤ `hw.perflevel0.physicalcpu` unique indices
4. Compare sensor keys against `src/temp.txt` M4 section

**Live system (M5 Pro, if available):**
1. Verify 3-phase runs (~55s)
2. Check "CPU Super Core" labels match `src/temp.txt` M5 section
3. Verify `hw.perflevel0.physicalcpu` matches Super core count in output
