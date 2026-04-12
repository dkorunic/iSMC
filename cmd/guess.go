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
	guessSampleCount    = 3
	guessSampleInterval = 500 * time.Millisecond
	// guessTempMin is set to a realistic lower bound for a running Mac. Hardware
	// reports (e.g. M1 Ultra) show a hard gap between ~12 °C (highest
	// sub-ambient/disconnected probe) and ~26 °C (lowest genuine ambient inlet).
	// 25 °C sits safely inside that gap: all real die/cluster sensors idle at ≥30 °C
	// while unconnected ambient probes (0–12 °C) and virtual thermal-zone sensors
	// (exactly 0 °C) are excluded.
	guessTempMin = float32(25.0)
	guessTempMax = float32(150.0)

	// guessOutputThreshold is the minimum delta (°C) for a sensor to appear in
	// the final output. Sensors below this value are silently dropped.
	// Set to ≈10 σ above measurement noise (sp78 resolution ≈0.25 °C).
	guessOutputThreshold = float32(1.5)

	// guessClusterRatio is the maximum relative gap between the P-phase and
	// E-phase deltas for a sensor still to be classified as a cluster/package
	// sensor rather than assigned to a specific phase.
	// If min(pDelta,eDelta)/max(pDelta,eDelta) >= (1−guessClusterRatio) the
	// sensor is treated as cluster-level (both phases equally hot).
	guessClusterRatio = float32(0.20)
)

var guessCmd = &cobra.Command{
	Use:   "guess",
	Short: "Map SMC temperature sensors to CPU cores by thermal correlation",
	Long: `guess stresses all logical CPU cores simultaneously — twice, once with
QOS_CLASS_USER_INITIATED (biases the OS toward P-cores) and once with
QOS_CLASS_BACKGROUND (biases toward E-cores) — and correlates the resulting
temperature rise in T-prefixed SMC sensors to produce a sensor mapping list
in the format used by src/temp.txt.

Within each phase, per-core labels are derived from SMC key naming patterns:
keys sharing the same non-numeric structure (e.g. TC*c) form a series, sorted
by their numeric index, and are assigned sequential core labels.

The process takes roughly 34 seconds on any chip.
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

// deltaTemps returns sensors in hot that exceed the corresponding baseline value
// by at least guessOutputThreshold.
func deltaTemps(base, hot map[string]float32) map[string]float32 {
	d := make(map[string]float32)

	for k, b := range base {
		if h, ok := hot[k]; ok {
			if delta := h - b; delta >= guessOutputThreshold {
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

// runAllCoresPhase launches numCPU spinCore goroutines simultaneously under the
// given QoS class, stresses for guessStressDuration, samples temperatures at
// peak load, then returns the delta map relative to baseline.
func runAllCoresPhase(numCPU int, qosClass int, baseline map[string]float32) map[string]float32 {
	done := make(chan struct{})

	for i := range numCPU {
		go spinCore(i+1, qosClass, done)
	}

	time.Sleep(guessStressDuration)

	hot := avgRawTemps(guessSampleCount, guessSampleInterval)

	close(done)

	responded := 0
	deltas := deltaTemps(baseline, hot)

	for _, d := range deltas {
		if d >= guessOutputThreshold {
			responded++
		}
	}

	fmt.Printf("  →  %d sensor(s) responded\n", responded)

	return deltas
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
	fmt.Printf("Per-phase: %v stress  (×2 phases, all %d cores simultaneously)\n\n",
		guessStressDuration, numCPU)

	fmt.Print("Sampling baseline... ")

	baseline := avgRawTemps(guessSampleCount, guessSampleInterval)

	fmt.Printf("%d sensors visible.\n\n", len(baseline))

	// Phase 1: all cores with QOS_CLASS_USER_INITIATED (biases toward P-cores).
	fmt.Println("── Phase 1: P-core sweep (QOS_CLASS_USER_INITIATED) ──")
	fmt.Printf("  Stressing all %d cores...", numCPU)
	pDeltas := runAllCoresPhase(numCPU, stress.QoSUserInitiated, baseline)

	// Inter-phase cooldown and re-baseline.
	fmt.Printf("\n  Inter-phase cooldown (%v)...\n", guessCoolDuration)
	time.Sleep(guessCoolDuration)

	baseline = avgRawTemps(guessSampleCount, guessSampleInterval)

	// Phase 2: all cores with QOS_CLASS_BACKGROUND (biases toward E-cores).
	fmt.Printf("\n── Phase 2: E-core sweep (QOS_CLASS_BACKGROUND) ──\n")
	fmt.Printf("  Stressing all %d cores...", numCPU)
	eDeltas := runAllCoresPhase(numCPU, stress.QoSBackground, baseline)

	printMapping(family, numCPU, pDeltas, eDeltas)
}

// printMapping analyses the two-phase delta results and emits a src/temp.txt-style
// mapping. Sensors are classified as P-cluster or E-cluster based on which phase
// produced the stronger response, then grouped by SMC key naming-pattern series
// and assigned sequential per-core labels.
func printMapping(family string, numCPU int, pDeltas, eDeltas map[string]float32) {
	// Union of all observed sensor keys across both phases.
	allKeys := make(map[string]struct{})

	for k := range pDeltas {
		allKeys[k] = struct{}{}
	}

	for k := range eDeltas {
		allKeys[k] = struct{}{}
	}

	var pKeys, eKeys, clusterKeys []string

	for key := range allKeys {
		pDelta := pDeltas[key]
		eDelta := eDeltas[key]

		dominant := pDelta
		if eDelta > pDelta {
			dominant = eDelta
		}

		if dominant < guessOutputThreshold {
			continue
		}

		// Cluster detection: both phases responded and neither clearly dominates
		// (relative gap is within guessClusterRatio).
		if pDelta >= guessOutputThreshold && eDelta >= guessOutputThreshold {
			lo, hi := pDelta, eDelta
			if lo > hi {
				lo, hi = hi, lo
			}

			if lo/hi >= (1 - guessClusterRatio) {
				clusterKeys = append(clusterKeys, key)

				continue
			}
		}

		if pDelta >= eDelta {
			pKeys = append(pKeys, key)
		} else {
			eKeys = append(eKeys, key)
		}
	}

	sort.Strings(clusterKeys)

	// ── Header ──────────────────────────────────────────────────────────────────
	fmt.Println()
	fmt.Printf("// %s (%d logical CPUs) – guessed sensor mappings\n", family, numCPU)
	fmt.Println("// WARNING: automated correlation is approximate; always verify on real hardware.")
	fmt.Printf("// %s\n", strings.Repeat("─", 72))

	// ── Performance cores (P-phase sensors) ─────────────────────────────────────
	pGroups := groupBySeries(pKeys)
	pSeriesKeys := sortedSeriesKeys(pGroups)

	if len(pSeriesKeys) == 0 {
		fmt.Println("// WARNING: Phase 1 (P-cores) produced no sensor responses above threshold.")
		fmt.Println("// Run on an idle machine or increase stress duration.")
	}

	coreIdx := 1

	for _, sk := range pSeriesKeys {
		sensors := pGroups[sk]
		fmt.Printf("// Series %-6s → %d sensor(s) → CPU Performance Core %d..%d\n",
			sk, len(sensors), coreIdx, coreIdx+len(sensors)-1)

		for _, k := range sensors {
			fmt.Printf("CPU Performance Core %d:%s:%s\n", coreIdx, k, family)

			coreIdx++
		}
	}

	// ── Efficiency cores (E-phase sensors) ──────────────────────────────────────
	eGroups := groupBySeries(eKeys)
	eSeriesKeys := sortedSeriesKeys(eGroups)

	if len(eSeriesKeys) == 0 && numCPU > 1 {
		fmt.Println("// WARNING: Phase 2 (E-cores) produced no sensor responses above threshold.")
		fmt.Println("// Run on an idle machine or increase stress duration.")
	}

	eIdx := 1

	for _, sk := range eSeriesKeys {
		sensors := eGroups[sk]
		fmt.Printf("// Series %-6s → %d sensor(s) → CPU Efficiency Core %d..%d\n",
			sk, len(sensors), eIdx, eIdx+len(sensors)-1)

		for _, k := range sensors {
			fmt.Printf("CPU Efficiency Core %d:%s:%s\n", eIdx, k, family)

			eIdx++
		}
	}

	// ── Cluster / package sensors ────────────────────────────────────────────────
	if len(clusterKeys) > 0 {
		fmt.Println()
		fmt.Printf("// %s\n", strings.Repeat("─", 72))
		fmt.Printf("// Cluster/package sensors (both phases within %.0f%% of each other):\n",
			guessClusterRatio*100)

		for _, k := range clusterKeys {
			fmt.Printf("// %-6s  P-phase +%.1f°C  E-phase +%.1f°C\n", k, pDeltas[k], eDeltas[k])
		}
	}
}
