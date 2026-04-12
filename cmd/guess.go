// Copyright (C) 2026  Dinko Korunic
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, version 3.
//
// This program is distributed in the hope that it will be useful, but
// WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the GNU
// General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program. If not, see <http://www.gnu.org/licenses/>.

//go:build darwin

package cmd

import (
	"fmt"
	"math"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/dkorunic/iSMC/platform"
	"github.com/dkorunic/iSMC/smc"
	"github.com/dkorunic/iSMC/stress"
	"github.com/spf13/cobra"
)

const (
	guessStressDuration = 8 * time.Second
	guessSettleDuration = 2 * time.Second
	guessCoolDuration   = 12 * time.Second
	guessDeltaThreshold = float32(1.5) // °C minimum to count as a genuine response
	guessSampleCount    = 3
	guessSampleInterval = 500 * time.Millisecond
	guessTempMin        = float32(1.0)
	guessTempMax        = float32(150.0)
)

var guessCmd = &cobra.Command{
	Use:   "guess",
	Short: "Map SMC temperature sensors to CPU cores by thermal correlation",
	Long: `guess stresses each logical CPU core in sequence — twice, once with
QOS_CLASS_USER_INITIATED (biases the OS toward P-cores) and once with
QOS_CLASS_BACKGROUND (biases toward E-cores) — and correlates the resulting
temperature rise in T-prefixed SMC sensors to produce a sensor mapping list
in the format used by src/temp.txt.

The process takes roughly 2 × (stressDuration + cooldown) × NumCPU ≈ 10–20 min.
Run on an otherwise-idle machine for best results.`,
	Run: runGuess,
}

func init() {
	rootCmd.AddCommand(guessCmd)
}

// rawTemps reads every T-prefixed SMC key and returns a map of key → °C for all
// sensors that report a plausible temperature value.
func rawTemps() map[string]float32 {
	out := make(map[string]float32)

	for _, k := range smc.GetRaw() {
		if len(k.Key) == 0 || k.Key[0] != 'T' {
			continue
		}

		v, ok := smc.RawKeyToFloat32(k)
		if !ok || v < guessTempMin || v > guessTempMax {
			continue
		}

		out[k.Key] = v
	}

	return out
}

// avgRawTemps returns per-key averages over n samples taken sampleInterval apart.
func avgRawTemps(n int, interval time.Duration) map[string]float32 {
	sums := make(map[string]float64)
	counts := make(map[string]int)

	for i := range n {
		if i > 0 {
			time.Sleep(interval)
		}

		for k, v := range rawTemps() {
			sums[k] += float64(v)
			counts[k]++
		}
	}

	result := make(map[string]float32, len(sums))
	for k, s := range sums {
		result[k] = float32(s / float64(counts[k]))
	}

	return result
}

// deltaTemps returns only those sensors in hot that exceed the corresponding baseline
// value by at least guessDeltaThreshold.
func deltaTemps(base, hot map[string]float32) map[string]float32 {
	d := make(map[string]float32)

	for k, b := range base {
		if h, ok := hot[k]; ok {
			if delta := h - b; delta >= guessDeltaThreshold {
				d[k] = delta
			}
		}
	}

	return d
}

// spinCore locks the goroutine to an OS thread, sets the QoS class to bias the OS
// scheduler toward the desired core type, sets a macOS thread-affinity tag to prefer
// a specific hardware thread within that type, then burns CPU until done is closed.
func spinCore(affinityTag int, qosClass int, done <-chan struct{}) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	// QoS must be set before affinity so the scheduler applies the class when
	// it first considers where to place this thread.
	stress.SetQoS(qosClass)
	stress.SetAffinityTag(affinityTag)

	// Four independent FMA+sqrt chains saturate all FP execution units simultaneously.
	// Apple Silicon P-cores (Everest) have four FP ports; a single chain leaves three
	// idle. math.FMA(a, a, c) lowers to FMADD on arm64 (fused multiply-add, one rounding)
	// so each chain step is two instructions: FMADD + FSQRT. Because a, b, c, d carry no
	// cross-chain data dependencies the out-of-order engine dispatches all four in parallel.
	//
	// Growth: a[n] ≈ √(n·π + a[0]²), so after 2 500 inner steps a ≈ 89 — well below 1e15.
	// The overflow guards below are a safety net; they never fire during normal stress runs.
	a, b, c, d := 1.0001, 1.0003, 1.0007, 1.0013

	for {
		for range 2_500 {
			a = math.Sqrt(math.FMA(a, a, math.Pi))
			b = math.Sqrt(math.FMA(b, b, math.E))
			c = math.Sqrt(math.FMA(c, c, math.Sqrt2))
			d = math.Sqrt(math.FMA(d, d, math.Phi))
		}

		// Safety resets outside the hot loop — no per-iteration branch in the inner path.
		if a > 1e15 {
			a = 1.0001
		}

		if b > 1e15 {
			b = 1.0003
		}

		if c > 1e15 {
			c = 1.0007
		}

		if d > 1e15 {
			d = 1.0013
		}

		select {
		case <-done:
			return
		default:
		}
	}
}

// sensorResult aggregates cross-core observations for a single SMC sensor key.
type sensorResult struct {
	key       string
	bestCore  int     // 0-based index of the core that caused the largest delta
	bestDelta float32 // highest delta observed across all core tests
	hits      int     // number of distinct cores that triggered this sensor
}

// runPhase stresses each logical core with the given QoS class, measures temperature
// deltas against the provided baseline, and returns per-core delta maps.
// The baseline is re-sampled after each inter-core cooldown to compensate for ambient
// drift over the long measurement window.
func runPhase(numCPU int, qosClass int, baseline map[string]float32) []map[string]float32 {
	allDeltas := make([]map[string]float32, numCPU)

	for i := range numCPU {
		fmt.Printf("  Core %2d/%d  stressing", i+1, numCPU)

		done := make(chan struct{})
		go spinCore(i+1, qosClass, done)

		time.Sleep(guessStressDuration)
		close(done)
		time.Sleep(guessSettleDuration)

		hot := avgRawTemps(guessSampleCount, guessSampleInterval)
		deltas := deltaTemps(baseline, hot)
		allDeltas[i] = deltas

		fmt.Printf("  →  %d sensor(s) responded\n", len(deltas))

		if i < numCPU-1 {
			fmt.Printf("             cooling  (%v)\n", guessCoolDuration)
			time.Sleep(guessCoolDuration)

			// Re-baseline after each cooldown to compensate for ambient drift.
			baseline = avgRawTemps(guessSampleCount, guessSampleInterval)
		}
	}

	return allDeltas
}

// runGuess implements the guess subcommand.
func runGuess(_ *cobra.Command, _ []string) {
	numCPU := runtime.NumCPU()
	family := platform.GetFamily()

	if family == "" || family == "Unknown" {
		switch runtime.GOARCH {
		case "arm64":
			family = "Apple"
		default:
			family = "Intel"
		}

		fmt.Printf("Warning: chip family not detected; using %q as platform tag.\n\n", family)
	}

	fmt.Printf("Platform : %s\n", family)
	fmt.Printf("CPUs     : %d logical\n", numCPU)
	fmt.Printf("Per-core : %v stress + %v settle + %v cooldown  (×2 phases)\n\n",
		guessStressDuration, guessSettleDuration, guessCoolDuration)

	fmt.Print("Sampling baseline... ")
	baseline := avgRawTemps(guessSampleCount, guessSampleInterval)
	fmt.Printf("%d sensors visible.\n\n", len(baseline))

	// Phase 1: P-core sweep — QOS_CLASS_USER_INITIATED biases the scheduler toward P-cores.
	fmt.Println("── Phase 1: P-core sweep (QOS_CLASS_USER_INITIATED) ──")
	pAllDeltas := runPhase(numCPU, stress.QoSUserInitiated, baseline)

	// Inter-phase cooldown and re-baseline before the E-core sweep.
	fmt.Printf("\n  Inter-phase cooldown (%v)...\n", guessCoolDuration)
	time.Sleep(guessCoolDuration)
	baseline = avgRawTemps(guessSampleCount, guessSampleInterval)

	// Phase 2: E-core sweep — QOS_CLASS_BACKGROUND biases the scheduler toward E-cores.
	fmt.Printf("\n── Phase 2: E-core sweep (QOS_CLASS_BACKGROUND) ──\n")
	eAllDeltas := runPhase(numCPU, stress.QoSBackground, baseline)

	printMapping(family, numCPU, pAllDeltas, eAllDeltas)
}

// buildByKey aggregates per-core delta maps into a per-sensor result map, tracking
// the best (maximum) delta and which core produced it.
func buildByKey(allDeltas []map[string]float32) map[string]*sensorResult {
	byKey := make(map[string]*sensorResult)

	for coreIdx, deltas := range allDeltas {
		for key, d := range deltas {
			r, ok := byKey[key]
			if !ok {
				r = &sensorResult{key: key}
				byKey[key] = r
			}

			r.hits++

			if d > r.bestDelta {
				r.bestDelta = d
				r.bestCore = coreIdx
			}
		}
	}

	return byKey
}

// activeSortedCores returns a slice of core indices present in coreMap, sorted by
// peak delta descending (hottest / most-responsive first).
func activeSortedCores(coreMap map[int][]*sensorResult) []int {
	cores := make([]int, 0, len(coreMap))
	for c := range coreMap {
		cores = append(cores, c)
	}

	sort.Slice(cores, func(a, b int) bool {
		return peakDelta(coreMap[cores[a]]) > peakDelta(coreMap[cores[b]])
	})

	return cores
}

// printMapping analyses the two-phase delta results and emits a src/temp.txt-style
// mapping. Each sensor is classified as a P-core, E-core, or cluster/package sensor
// based on which QoS phase produced the stronger thermal response, replacing the
// previous delta-magnitude heuristic with scheduler-guided classification.
func printMapping(family string, numCPU int, pAllDeltas, eAllDeltas []map[string]float32) {
	pByKey := buildByKey(pAllDeltas)
	eByKey := buildByKey(eAllDeltas)

	// Union of all observed sensor keys across both phases.
	allKeys := make(map[string]struct{})
	for k := range pByKey {
		allKeys[k] = struct{}{}
	}

	for k := range eByKey {
		allKeys[k] = struct{}{}
	}

	pCoreMap := make(map[int][]*sensorResult) // P-phase sensors by bestCore
	eCoreMap := make(map[int][]*sensorResult) // E-phase sensors by bestCore
	var clusterSensors []*sensorResult

	for key := range allKeys {
		pr := pByKey[key] // nil if absent in P-phase
		er := eByKey[key] // nil if absent in E-phase

		pHits := 0
		if pr != nil {
			pHits = pr.hits
		}

		eHits := 0
		if er != nil {
			eHits = er.hits
		}

		// A sensor responding to more than half the cores in either phase almost
		// certainly reflects a shared cluster or package measurement.
		if numCPU > 1 && (pHits*2 > numCPU || eHits*2 > numCPU) {
			r := pr
			if r == nil || (er != nil && er.bestDelta > r.bestDelta) {
				r = er
			}

			clusterSensors = append(clusterSensors, r)
			continue
		}

		pDelta := float32(0)
		if pr != nil {
			pDelta = pr.bestDelta
		}

		eDelta := float32(0)
		if er != nil {
			eDelta = er.bestDelta
		}

		// Assign to the phase that produced the stronger signal. P-phase wins on ties
		// since performance cores typically run hotter.
		if pDelta >= eDelta && pr != nil {
			pCoreMap[pr.bestCore] = append(pCoreMap[pr.bestCore], pr)
		} else if er != nil {
			eCoreMap[er.bestCore] = append(eCoreMap[er.bestCore], er)
		}
	}

	// Sort within each core group by key name for deterministic output.
	for _, ss := range pCoreMap {
		sort.Slice(ss, func(a, b int) bool { return ss[a].key < ss[b].key })
	}

	for _, ss := range eCoreMap {
		sort.Slice(ss, func(a, b int) bool { return ss[a].key < ss[b].key })
	}

	sort.Slice(clusterSensors, func(a, b int) bool { return clusterSensors[a].key < clusterSensors[b].key })

	pActiveCores := activeSortedCores(pCoreMap)
	eActiveCores := activeSortedCores(eCoreMap)

	// ── Header ──────────────────────────────────────────────────────────────────
	fmt.Println()
	fmt.Printf("// %s (%d logical CPUs) – guessed sensor mappings\n", family, numCPU)
	fmt.Println("// WARNING: automated correlation is approximate; always verify on real hardware.")
	fmt.Printf("// %s\n", strings.Repeat("─", 72))

	// ── Performance cores (P-phase sensors) ─────────────────────────────────────
	pIdx := 1
	for _, c := range pActiveCores {
		ss := pCoreMap[c]
		peak := peakDelta(ss)
		label := fmt.Sprintf("CPU Performance Core %d", pIdx)
		pIdx++

		fmt.Printf("// Logical CPU %d → peak +%.1f°C → %s\n", c+1, peak, label)

		for _, r := range ss {
			fmt.Printf("%s:%s:%s\n", label, r.key, family)
		}
	}

	// ── Efficiency cores (E-phase sensors) ──────────────────────────────────────
	eIdx := 1
	for _, c := range eActiveCores {
		ss := eCoreMap[c]
		peak := peakDelta(ss)
		label := fmt.Sprintf("CPU Efficiency Core %d", eIdx)
		eIdx++

		fmt.Printf("// Logical CPU %d → peak +%.1f°C → %s\n", c+1, peak, label)

		for _, r := range ss {
			fmt.Printf("%s:%s:%s\n", label, r.key, family)
		}
	}

	// ── Cluster / package sensors ────────────────────────────────────────────────
	if len(clusterSensors) > 0 {
		fmt.Println()
		fmt.Printf("// %s\n", strings.Repeat("─", 72))
		fmt.Printf("// Cluster/package sensors (responded to >%d/%d cores in a single phase):\n",
			numCPU/2, numCPU)

		for _, r := range clusterSensors {
			fmt.Printf("// %-6s  peak +%.1f°C\n", r.key, r.bestDelta)
		}
	}

	// ── Cores with no response ───────────────────────────────────────────────────
	silent := make([]string, 0)

	for c := range numCPU {
		if len(pCoreMap[c]) == 0 && len(eCoreMap[c]) == 0 {
			silent = append(silent, fmt.Sprintf("%d", c+1))
		}
	}

	if len(silent) > 0 {
		fmt.Println()
		fmt.Printf("// %s\n", strings.Repeat("─", 72))
		fmt.Printf("// No sensor responded above threshold for logical CPU(s): %s\n",
			strings.Join(silent, ", "))
		fmt.Println("// Consider re-running on an idle machine or increasing stress duration.")
	}
}

// peakDelta returns the maximum bestDelta among the given sensor results.
func peakDelta(ss []*sensorResult) float32 {
	var m float32
	for _, r := range ss {
		if r.bestDelta > m {
			m = r.bestDelta
		}
	}

	return m
}
