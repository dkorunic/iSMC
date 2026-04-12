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
	"strconv"
	"strings"
	"time"

	"github.com/dkorunic/iSMC/platform"
	"github.com/dkorunic/iSMC/smc"
	"github.com/dkorunic/iSMC/stress"
	"github.com/spf13/cobra"
)

const (
	guessStressDuration = 8 * time.Second
	guessCoolDuration   = 12 * time.Second
	guessDeltaThreshold = float32(1.5) // °C minimum to count as a genuine response
	guessSampleCount    = 3
	guessSampleInterval = 500 * time.Millisecond
	// guessTempMin is set to a realistic lower bound for a running Mac. Hardware
	// reports (e.g. M1 Ultra) show a hard gap between ~12 °C (highest
	// sub-ambient/disconnected probe) and ~26 °C (lowest genuine ambient inlet).
	// 25 °C sits safely inside that gap: all real die/cluster sensors idle at ≥30 °C
	// while unconnected ambient probes (0–12 °C) and virtual thermal-zone sensors
	// (exactly 0 °C) are excluded. Raising from 20 °C to 25 °C closes any edge cases
	// without risk of dropping a real sensor on any shipping Apple Silicon chip.
	guessTempMin = float32(25.0)
	guessTempMax = float32(150.0)

	// guessTrackThreshold is the minimum delta used when collecting cross-core
	// responses. It is intentionally lower than guessOutputThreshold so that
	// sub-threshold thermal cross-talk to neighbouring cores is captured and counted
	// toward a sensor's hit score. Without this, a broadly coupled sensor whose
	// secondary-core responses all fall below the output threshold would appear with
	// hits=1 and bypass the exclusivity check.
	//
	// Set to ≈3 σ above measurement noise (sp78 resolution ≈0.25 °C, noise after
	// 3-sample average ≈0.15 °C RMS).
	guessTrackThreshold = float32(0.5)

	// guessOutputThreshold is the minimum best-core delta for a sensor to appear in
	// the final per-core or cluster output. Sensors tracked at guessTrackThreshold
	// that never exceed this value are used only for exclusivity analysis and are
	// silently dropped from the mapping.
	guessOutputThreshold = float32(1.5)

	// guessMaxCoreHits is the absolute upper bound on the number of distinct logical
	// cores a sensor may respond to (at guessTrackThreshold) before it is reclassified
	// as a cluster/package sensor. Physical per-core sensors may bleed heat into one
	// immediate neighbour (hits=2) but responding to three or more separate cores
	// indicates shared thermal mass independent of chip size.
	guessMaxCoreHits = 2

	// guessExclusivityRatio is the minimum (best-core delta) / (second-best-core delta)
	// required to keep a sensor as per-core. If the second-best response is more than
	// half the best response the sensor lacks a clear dominant association with one
	// core and is reclassified as a cluster sensor.
	guessExclusivityRatio = float32(2.0)
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

// deltaTemps returns sensors in hot that exceed the corresponding baseline value by at
// least guessTrackThreshold. The lower tracking threshold (vs guessOutputThreshold)
// captures sub-threshold cross-core responses so that broadly coupled sensors
// accumulate a high hit count and are later reclassified as cluster sensors.
func deltaTemps(base, hot map[string]float32) map[string]float32 {
	d := make(map[string]float32)

	for k, b := range base {
		if h, ok := hot[k]; ok {
			if delta := h - b; delta >= guessTrackThreshold {
				d[k] = delta
			}
		}
	}

	return d
}

// seriesKey returns the SMC key with every decimal digit replaced by '*'.
// Keys sharing a series key differ only in their numeric index and belong to
// the same per-core sensor series (e.g. "TC0c", "TC3c" → "TC*c").
func seriesKey(key string) string {
	b := []byte(key)

	for i, c := range b {
		if c >= '0' && c <= '9' {
			b[i] = '*'
		}
	}

	return string(b)
}

// numericValue extracts all decimal digit characters from key in order and
// parses them as a decimal integer. Returns 0 for keys with no digits.
// "TC3c" → 3, "Tp09" → 9, "Te12" → 12, "TcXX" → 0.
func numericValue(key string) int {
	var digits []byte

	for _, c := range []byte(key) {
		if c >= '0' && c <= '9' {
			digits = append(digits, c)
		}
	}

	if len(digits) == 0 {
		return 0
	}

	v, _ := strconv.Atoi(string(digits))

	return v
}

// groupBySeries groups SMC sensor keys by series key (non-numeric pattern).
// Within each group the keys are sorted ascending by numericValue so that
// sorted position 0 maps to Core 1, position 1 maps to Core 2, etc.
func groupBySeries(keys []string) map[string][]string {
	groups := make(map[string][]string)

	for _, k := range keys {
		sk := seriesKey(k)
		groups[sk] = append(groups[sk], k)
	}

	for sk := range groups {
		sort.Slice(groups[sk], func(a, b int) bool {
			return numericValue(groups[sk][a]) < numericValue(groups[sk][b])
		})
	}

	return groups
}

// sortedSeriesKeys returns the keys of a series group map sorted lexicographically.
func sortedSeriesKeys(groups map[string][]string) []string {
	keys := make([]string, 0, len(groups))

	for k := range groups {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	return keys
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
	key             string
	bestCore        int     // 0-based index of the core that caused the largest delta
	bestDelta       float32 // highest delta observed across all core tests
	secondBestDelta float32 // delta of the second-most-responsive core (0 if hits==1)
	hits            int     // number of distinct cores that triggered this sensor
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

		// Sample at peak: measure while the core is still under load. Apple Silicon sheds
		// heat within seconds of dropping load, so measuring after close(done) would yield
		// near-baseline readings and zero deltas above the 1.5 °C threshold.
		time.Sleep(guessStressDuration)
		hot := avgRawTemps(guessSampleCount, guessSampleInterval)
		close(done)
		deltas := deltaTemps(baseline, hot)
		allDeltas[i] = deltas

		// Count only sensors that crossed the output threshold for the progress line;
		// sub-threshold entries are present in deltas solely for exclusivity analysis.
		responded := 0
		for _, d := range deltas {
			if d >= guessOutputThreshold {
				responded++
			}
		}

		fmt.Printf("  →  %d sensor(s) responded\n", responded)

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
	fmt.Printf("Per-core : %v stress + %v cooldown  (×2 phases)\n\n",
		guessStressDuration, guessCoolDuration)

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
// the best and second-best deltas across all cores.
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
				// Demote current best to second-best before overwriting.
				r.secondBestDelta = r.bestDelta
				r.bestDelta = d
				r.bestCore = coreIdx
			} else if d > r.secondBestDelta {
				r.secondBestDelta = d
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

// isClusterSensor returns true when r should be treated as a cluster/package sensor
// rather than a per-core sensor. Three independent conditions trigger reclassification:
//
//  1. Majority response: the sensor responded to more than half the logical CPUs —
//     it is almost certainly measuring shared die or cluster thermal mass.
//  2. Absolute hit cap: the sensor responded to more than guessMaxCoreHits distinct
//     cores, which indicates cross-cluster heat diffusion regardless of chip size.
//  3. Lack of exclusivity: the best-core delta is less than guessExclusivityRatio times
//     the second-best delta, meaning no single core clearly "owns" the sensor.
func isClusterSensor(r *sensorResult, numCPU int) bool {
	if r == nil {
		return false
	}

	if numCPU > 1 && r.hits*2 > numCPU {
		return true
	}

	if r.hits > guessMaxCoreHits {
		return true
	}

	if r.hits > 1 && r.secondBestDelta > 0 && r.bestDelta < r.secondBestDelta*guessExclusivityRatio {
		return true
	}

	return false
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

		// Reclassify as cluster if either phase flags the sensor as non-exclusive.
		if isClusterSensor(pr, numCPU) || isClusterSensor(er, numCPU) {
			r := pr
			if r == nil || (er != nil && er.bestDelta > r.bestDelta) {
				r = er
			}

			// Only surface cluster sensors whose best delta crossed the output threshold;
			// sensors tracked solely at the sub-threshold level are silently dropped.
			if r.bestDelta >= guessOutputThreshold {
				clusterSensors = append(clusterSensors, r)
			}

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

		// Assign to the phase with the stronger signal (P-phase wins ties).
		// Drop the sensor entirely if the winning delta never crossed the output
		// threshold — it was tracked only for exclusivity analysis.
		if pDelta >= eDelta && pr != nil {
			if pr.bestDelta >= guessOutputThreshold {
				pCoreMap[pr.bestCore] = append(pCoreMap[pr.bestCore], pr)
			}
		} else if er != nil {
			if er.bestDelta >= guessOutputThreshold {
				eCoreMap[er.bestCore] = append(eCoreMap[er.bestCore], er)
			}
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
