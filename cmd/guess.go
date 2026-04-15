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
	Long: `guess stresses all logical CPU cores simultaneously — in two or three phases
depending on chip topology — and correlates the resulting temperature rise in
T-prefixed SMC sensors to produce a sensor mapping list in the format used by
src/temp.txt.

On 2-tier chips (M1–M4) the phases are QOS_CLASS_USER_INITIATED (biases the OS
toward P-cores) and QOS_CLASS_BACKGROUND (biases toward E-cores). On 3-tier chips
(M5+) a third phase with QOS_CLASS_USER_INTERACTIVE precedes the others to bias
toward Super cores.

Within each phase, per-core labels are derived from SMC key naming patterns:
keys sharing the same non-numeric structure (e.g. TC*c) form a series, then
stride/gap analysis within each series groups sensors that belong to the same
physical core into a single sub-group.

The process takes roughly 34 seconds on 2-tier chips, or ~55 seconds on 3-tier chips.
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

// seriesKey returns the SMC key with every decimal digit and hex digit (A-F)
// in the numeric index replaced by '*'. Keys sharing a series key differ only
// in their numeric index and belong to the same per-core sensor series
// (e.g. "TC0c", "TC3c" → "TC*c", "Tp0A", "Tp0C" → "Tp**").
func seriesKey(key string) string {
	b := []byte(key)
	indexStarted := false

	for i, c := range b {
		switch {
		case c >= '0' && c <= '9':
			indexStarted = true
			b[i] = '*'
		case indexStarted && c >= 'A' && c <= 'F':
			// Continue masking hex digits that are part of the index
			b[i] = '*'
		case indexStarted && (c < 'A' || c > 'F'):
			// Stop masking once we hit a non-hex character after index started
			indexStarted = false
		}
	}

	return string(b)
}

// numericValue extracts the numeric index from an SMC key. The index starts at
// the first digit and includes all subsequent digits and uppercase hex digits (A-F).
// Lowercase letters are treated as series-key components and excluded. If any
// uppercase hex digits (A-F) are found in the index, parses as hexadecimal;
// otherwise parses as decimal. Returns 0 for keys with no digits.
// "TC3c" → 3, "Tp09" → 9, "Te12" → 12, "Tp0A" → 10, "TcXX" → 0.
func numericValue(key string) int {
	var digits []byte

	hasHexDigit := false
	indexStarted := false

	for _, c := range []byte(key) {
		if c >= '0' && c <= '9' {
			// First digit marks the start of the index
			indexStarted = true

			digits = append(digits, c)
		} else if indexStarted {
			// Once index has started, continue to collect uppercase hex digits
			if c >= 'A' && c <= 'F' {
				digits = append(digits, c)
				hasHexDigit = true
			} else {
				// Non-hex character after index started; stop collecting
				break
			}
		}
	}

	if len(digits) == 0 {
		return 0
	}

	if hasHexDigit {
		v, _ := strconv.ParseInt(string(digits), 16, 64)

		return int(v)
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

// groupByStrideWithinSeries splits a series' sorted sensor list into sub-groups
// based on gaps between consecutive numericValues.
//
// Rules:
//   - If all consecutive differences are equal (uniform stride): each sensor is
//     its own group. This covers M5-style single-sensor-per-core series:
//     Tp00/04/08 (stride 4) → [[Tp00],[Tp04],[Tp08]] → 3 cores.
//   - Otherwise (non-uniform gaps): split whenever the gap exceeds minDiff.
//     This covers M1/M3 triplets: diffs [1,1,2,1,1,2,...] → minDiff=1,
//     split at every 2 → [[Tp00,Tp01,Tp02],[Tp04,Tp05,Tp06],...] → N cores × 3.
//   - A single-element slice is returned as one group.
func groupByStrideWithinSeries(sensors []string) [][]string {
	if len(sensors) <= 1 {
		result := make([][]string, len(sensors))

		for i, s := range sensors {
			result[i] = []string{s}
		}

		return result
	}

	diffs := make([]int, len(sensors)-1)

	for i := 1; i < len(sensors); i++ {
		d := numericValue(sensors[i]) - numericValue(sensors[i-1])
		if d < 0 {
			d = -d
		}

		diffs[i-1] = d
	}

	uniform := true

	for _, d := range diffs[1:] {
		if d != diffs[0] {
			uniform = false

			break
		}
	}

	if uniform {
		groups := make([][]string, len(sensors))

		for i, s := range sensors {
			groups[i] = []string{s}
		}

		return groups
	}

	minDiff := diffs[0]

	for _, d := range diffs[1:] {
		if d < minDiff {
			minDiff = d
		}
	}

	var groups [][]string

	current := []string{sensors[0]}

	for i, d := range diffs {
		if d > minDiff {
			groups = append(groups, current)
			current = []string{sensors[i+1]}
		} else {
			current = append(current, sensors[i+1])
		}
	}

	return append(groups, current)
}

// phaseSpec describes one stress phase: its label prefix, QoS class, and expected
// physical core count from platform topology data.
type phaseSpec struct {
	label string // e.g. "CPU Super Core", "CPU Performance Core", "CPU Efficiency Core"
	qos   int    // QoS class constant from stress package
	cores int    // physicalCPU for this tier (0 if unknown)
}

// phaseResult pairs a phase specification with the sensor deltas it produced.
type phaseResult struct {
	deltas map[string]float32
	spec   phaseSpec
}

// phaseMidWord returns the middle word of a phase label for use in progress messages.
// "CPU Super Core" → "Super", "CPU Performance Core" → "Performance",
// "CPU Efficiency Core" → "Efficiency".
func phaseMidWord(label string) string {
	parts := strings.Fields(label)
	if len(parts) >= 2 {
		return parts[1]
	}

	return label
}

// qosName returns a short human-readable name for the given macOS QoS class constant.
func qosName(qos int) string {
	switch qos {
	case stress.QoSUserInteractive:
		return "UserInteractive"
	case stress.QoSUserInitiated:
		return "UserInitiated"
	case stress.QoSBackground:
		return "Background"
	default:
		return fmt.Sprintf("0x%02X", qos)
	}
}

// buildPhases constructs the ordered list of stress phases from live topology data.
// For 3-level chips (M5+): Super → Performance → Efficiency.
// For 2-level chips (M1–M4) or nil: Performance → Efficiency.
func buildPhases(perfLevels []platform.PerfLevel) []phaseSpec {
	switch len(perfLevels) {
	case 3:
		return []phaseSpec{
			{label: "CPU Super Core", qos: stress.QoSUserInteractive, cores: perfLevels[0].PhysicalCPU},
			{label: "CPU Performance Core", qos: stress.QoSUserInitiated, cores: perfLevels[1].PhysicalCPU},
			{label: "CPU Efficiency Core", qos: stress.QoSBackground, cores: perfLevels[2].PhysicalCPU},
		}
	default:
		pcpu, ecpu := 0, 0
		if len(perfLevels) == 2 {
			pcpu = perfLevels[0].PhysicalCPU
			ecpu = perfLevels[1].PhysicalCPU
		}

		return []phaseSpec{
			{label: "CPU Performance Core", qos: stress.QoSUserInitiated, cores: pcpu},
			{label: "CPU Efficiency Core", qos: stress.QoSBackground, cores: ecpu},
		}
	}
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

	deltas := deltaTemps(baseline, hot)

	fmt.Printf("  →  %d sensor(s) responded\n", len(deltas))

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

	perfLevels := platform.GetPerfLevels()
	phases := buildPhases(perfLevels)

	fmt.Printf("Platform : %s\n", family)
	fmt.Printf("CPUs     : %d logical\n", numCPU)
	fmt.Printf("Per-phase: %v stress  (×%d phases, all %d cores simultaneously)\n\n",
		guessStressDuration, len(phases), numCPU)

	if len(perfLevels) > 0 {
		fmt.Printf("Topology : %d perf level(s)\n", len(perfLevels))

		for i, pl := range perfLevels {
			fmt.Printf("           perflevel%d: %d phys-CPU %q\n", i, pl.PhysicalCPU, pl.Name)
		}

		fmt.Println()
	}

	fmt.Print("Sampling baseline... ")

	baseline := avgRawTemps(guessSampleCount, guessSampleInterval)

	fmt.Printf("%d sensors visible.\n\n", len(baseline))

	results := make([]phaseResult, 0, len(phases))

	for i, phase := range phases {
		fmt.Printf("── Phase %d: %s sweep (%s QoS) ──\n",
			i+1, phaseMidWord(phase.label), qosName(phase.qos))
		fmt.Printf("  Stressing all %d cores...", numCPU)

		deltas := runAllCoresPhase(numCPU, phase.qos, baseline)
		results = append(results, phaseResult{spec: phase, deltas: deltas})

		if i < len(phases)-1 {
			fmt.Printf("\n  Inter-phase cooldown (%v)...\n\n", guessCoolDuration)
			time.Sleep(guessCoolDuration)

			baseline = avgRawTemps(guessSampleCount, guessSampleInterval)
		}
	}

	printMapping(family, numCPU, perfLevels, results)
}

// printMapping analyses N-phase delta results and emits a src/temp.txt-style mapping.
// Sensors are classified by dominant phase, then grouped by SMC key series and
// further split by stride/gap analysis to assign correct per-core indices.
func printMapping(family string, numCPU int, perfLevels []platform.PerfLevel, results []phaseResult) {
	// Union of all observed sensor keys across all phases.
	allKeys := make(map[string]struct{})

	for _, r := range results {
		for k := range r.deltas {
			allKeys[k] = struct{}{}
		}
	}

	// phaseKeys[i] holds keys whose dominant phase is results[i].
	phaseKeys := make([][]string, len(results))

	var clusterKeys []string

	for key := range allKeys {
		// Find index of dominant phase (highest delta ≥ threshold).
		dominantIdx := -1
		dominantDelta := float32(0)

		for i, r := range results {
			d := r.deltas[key]
			if d >= guessOutputThreshold && d > dominantDelta {
				dominantDelta = d
				dominantIdx = i
			}
		}

		if dominantIdx < 0 {
			continue
		}

		// Cluster detection: find second-best phase.
		secondDelta := float32(0)

		for i, r := range results {
			if i == dominantIdx {
				continue
			}

			d := r.deltas[key]
			if d >= guessOutputThreshold && d > secondDelta {
				secondDelta = d
			}
		}

		if secondDelta > 0 {
			lo, hi := secondDelta, dominantDelta
			if lo > hi {
				lo, hi = hi, lo
			}

			if lo/hi >= (1 - guessClusterRatio) {
				clusterKeys = append(clusterKeys, key)

				continue
			}
		}

		phaseKeys[dominantIdx] = append(phaseKeys[dominantIdx], key)
	}

	sort.Strings(clusterKeys)

	// ── Header ──────────────────────────────────────────────────────────────────
	fmt.Println()
	fmt.Printf("// %s (%d logical CPUs) – guessed sensor mappings\n", family, numCPU)

	if len(perfLevels) > 0 {
		fmt.Printf("// Topology: %d perf level(s)", len(perfLevels))

		for i, pl := range perfLevels {
			if i == 0 {
				fmt.Printf(" | perflevel%d %d phys-CPU %q\n", i, pl.PhysicalCPU, pl.Name)
			} else {
				fmt.Printf("//            %s| perflevel%d %d phys-CPU %q\n",
					strings.Repeat(" ", 10), i, pl.PhysicalCPU, pl.Name)
			}
		}
	}

	fmt.Println("// WARNING: automated correlation is approximate; always verify on real hardware.")
	fmt.Printf("// %s\n", strings.Repeat("─", 72))

	// ── Per-phase sensor sections ────────────────────────────────────────────────
	for i, r := range results {
		keys := phaseKeys[i]
		groups := groupBySeries(keys)
		seriesKeys := sortedSeriesKeys(groups)

		if len(seriesKeys) == 0 {
			fmt.Printf("// WARNING: Phase %d (%s) produced no sensor responses above threshold.\n",
				i+1, phaseMidWord(r.spec.label))
			fmt.Println("// Run on an idle machine or increase stress duration.")

			continue
		}

		coreIdx := 1

		for _, sk := range seriesKeys {
			sensors := groups[sk]
			subGroups := groupByStrideWithinSeries(sensors)

			fmt.Printf("// Series %-6s → %d sensor(s) in %d group(s) → %s %d..%d\n",
				sk, len(sensors), len(subGroups), r.spec.label, coreIdx, coreIdx+len(subGroups)-1)

			if len(subGroups) == 1 && len(subGroups[0]) > 3 {
				fmt.Printf("// NOTE: %d-sensor group; manual review recommended (check src/temp.txt)\n",
					len(subGroups[0]))
			}

			for _, sg := range subGroups {
				for _, k := range sg {
					fmt.Printf("%s %d:%s:%s\n", r.spec.label, coreIdx, k, family)
				}

				coreIdx++
			}
		}
	}

	// ── Cluster / package sensors ────────────────────────────────────────────────
	if len(clusterKeys) > 0 {
		fmt.Println()
		fmt.Printf("// %s\n", strings.Repeat("─", 72))
		fmt.Printf("// Cluster/package sensors (top two phases within %.0f%% of each other):\n",
			guessClusterRatio*100)

		for _, k := range clusterKeys {
			parts := make([]string, 0, len(results))

			for _, r := range results {
				if d := r.deltas[k]; d > 0 {
					parts = append(parts, fmt.Sprintf("%s +%.1f°C", phaseMidWord(r.spec.label), d))
				}
			}

			fmt.Printf("// %-6s  %s\n", k, strings.Join(parts, "  "))
		}
	}
}
