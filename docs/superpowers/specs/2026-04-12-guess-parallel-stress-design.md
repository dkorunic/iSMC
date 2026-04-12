# guess: parallel all-cores stress + series-based stride

**Date:** 2026-04-12  
**Status:** Approved  
**File:** `cmd/guess.go`

---

## Problem

The current `guess` command stresses one logical CPU core at a time (sequential per-core sweep), requiring `2 × numCPU × (8s + 12s)` ≈ 13–16 minutes on M1/M2 Ultra chips. The per-core attribution it produces is more granular than required; the actual goal is to identify which T-prefixed SMC sensors respond to CPU load and map them to per-core labels via naming-pattern analysis.

---

## Goal

Identify all CPU thermal sensors (cluster/package level) in a single simultaneous all-cores stress run per phase, then derive per-core labels from SMC key naming patterns (stride detection). Total run time ≈ 34s regardless of core count.

---

## Architecture

### Current flow
```
runGuess
  → runPhase (sequential: numCPU iterations)
      → spinCore (one goroutine per iteration)
      → 8s stress → sample → 12s cooldown → re-baseline
      → returns []map[string]float32   (per-core delta arrays)
  → printMapping (sensorResult aggregation, exclusivity analysis)
```

### New flow
```
runGuess
  → runAllCoresPhase (one shot)
      → numCPU × spinCore goroutines simultaneously
      → 8s stress → sample → close all
      → returns map[string]float32     (single flat delta map)
  → printMapping (cross-phase delta comparison → series grouping → sequential labels)
```

---

## Components

### Removed
- `runPhase` — replaced by `runAllCoresPhase`
- `sensorResult` struct — per-core aggregation no longer needed
- `buildByKey` — superseded by series grouping
- `isClusterSensor` — all responding sensors are cluster-level by definition
- `activeSortedCores` — no per-core maps
- `peakDelta` — no per-core sensor result slices

### Added
- `runAllCoresPhase(numCPU int, qosClass int, baseline map[string]float32) map[string]float32`
- `seriesKey(key string) string`
- `numericValue(key string) int`
- `groupBySeries(keys []string) map[string][]string`

### Unchanged
- `rawTemps`, `avgRawTemps`, `deltaTemps`, `spinCore`
- All stress/cooldown constants
- Output format: `Label:Key:Family` lines (paste-ready for `src/temp.txt`)

---

## `runAllCoresPhase`

```
1. Print progress header: "Phase N (P/E-cores)  stressing all M cores..."
2. Launch numCPU goroutines: spinCore(i+1, qosClass, done) for i in 0..numCPU-1
3. Sleep guessStressDuration
4. Sample: hot = avgRawTemps(guessSampleCount, guessSampleInterval)
5. close(done)
6. Compute deltas = deltaTemps(baseline, hot)
7. Count sensors above guessOutputThreshold, print "→  N sensor(s) responded"
8. Return deltas
```

---

## Naming Pattern Analysis

### `seriesKey`
Replace every decimal digit character (`0`–`9`) with `*`.

| Key    | Series key |
|--------|-----------|
| `TC0c` | `TC*c`    |
| `TC3c` | `TC*c`    |
| `Te1T` | `Te*T`    |
| `Tp01` | `Tp**`    |
| `Tf0c` | `Tf*c`    |

Keys sharing a series key belong to the same per-core series (differ only in numeric index). Different series keys on the same chip represent distinct sensor families (e.g. two dies on Ultra chips: `TC*c` and `Tf*c`).

### `numericValue`
Extract all decimal digit characters in order and parse as a decimal integer.  
`TC3c` → `3`, `Tp09` → `9`, `Te12` → `12`.

### `groupBySeries`
Given a slice of sensor keys, return `map[seriesKey][]key` where each value slice is sorted ascending by `numericValue`. Position in the sorted slice = 0-based core index for that series.

---

## `printMapping` (new signature)

```go
func printMapping(family string, numCPU int, pDeltas, eDeltas map[string]float32)
```

**Classification:**
1. Union all keys from both delta maps.
2. For each key: dominant phase = higher delta. P wins ties.
3. If both deltas exceed `guessOutputThreshold` AND `min(pDelta,eDelta)/max(pDelta,eDelta) >= (1 - guessClusterRatio)` (new constant: `guessClusterRatio = float32(0.20)`) → cluster/package sensor (comment-only output block).
4. If dominant phase delta < `guessOutputThreshold` → drop silently.

**Output per P-cluster series group** (series keys sorted lexicographically, sensors within each series sorted by `numericValue` ascending):
```
// Series TC*c  →  4 sensors  →  CPU Performance Core 1..4
CPU Performance Core 1:TC0c:M4
CPU Performance Core 2:TC1c:M4
CPU Performance Core 3:TC2c:M4
CPU Performance Core 4:TC3c:M4
```

**E-cluster** — same structure with `CPU Efficiency Core N` labels.

**Cluster/package block** — comment lines only, same as current:
```
// ────────────────
// Cluster/package sensors (responded in both phases):
// TcXX  peak +5.2°C
```

**No-response warning** — if a phase produced zero sensors above threshold, emit:
```
// WARNING: Phase N produced no sensor responses above threshold.
// Run on an idle machine or increase stress duration.
```

---

## Edge Cases

| Situation | Handling |
|-----------|----------|
| Series with 1 sensor | Valid: outputs `CPU Performance Core 1:Tkey:Family` |
| Sensor only in one phase | Belongs entirely to that phase, no cross-phase logic needed |
| Sensor in both phases, both above threshold, within 20% | Cluster sensor, comment block |
| Sensor in both phases, one clearly dominant (>20% difference) | Assigned to dominant phase |
| No sensors in a phase | Warning comment, skip that phase's output section |
| Numeric gap in series (TC0c, TC2c missing TC1c) | Sequential assignment: position 0 → Core 1, position 1 → Core 2 |

---

## Timing

| Step | Duration |
|------|----------|
| Initial baseline | 1.5s |
| Phase 1 stress | 8s |
| Phase 1 sampling | 1.5s |
| Cooldown + re-baseline | 13.5s |
| Phase 2 stress | 8s |
| Phase 2 sampling | 1.5s |
| **Total** | **~34s** |

(Current: 2 × 20 cores × 20s = **~13 min** on M1 Ultra)

---

## Long description update

The `guessCmd.Long` description should be updated to reflect the new two-phase simultaneous approach and the ~34s typical run time.
