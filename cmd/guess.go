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
	"github.com/dkorunic/iSMC/reports"
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
depending on the chip's pair signature — and correlates the resulting temperature
rise in T-prefixed SMC sensors to produce a sensor mapping list in the format used
by src/temp.txt.

Phase labels and QoS hints are driven by the SKU's pair signature (from the
validate-temp-mappings family roster):
  - P+E (M1–M4 family, A18 Pro):   Performance + Efficiency
  - S+E (M5 base):                 Super + Efficiency
  - S+P (M5 Pro / M5 Max):         Super + Performance
  - 3-level perflevel hierarchy:   Super + Performance + Efficiency

QOS_CLASS_USER_INTERACTIVE biases the kernel toward Super cores, USER_INITIATED
toward Performance cores, and BACKGROUND toward Efficiency cores.

Within each phase, per-core labels are derived from SMC key naming patterns:
keys sharing the same non-numeric structure (e.g. TC*c) form a series, then
stride/gap analysis within each series groups sensors that belong to the same
physical core into a single sub-group. The detected per-type counts are
cross-checked against the SKU's expected layout (e.g. M5 Pro: 5–6 Super + 10–12
Performance), and deviations are flagged inline so the operator knows which lines
need manual review before pasting into src/temp.txt.

The process takes roughly 34 seconds on 2-phase chips, or ~55 seconds on 3-phase
chips. Run on an otherwise-idle machine for best results.`,
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
			b[i] = '*'
		case indexStarted && (c < 'A' || c > 'F'):
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
			indexStarted = true

			digits = append(digits, c)
		} else if indexStarted {
			if c >= 'A' && c <= 'F' {
				digits = append(digits, c)
				hasHexDigit = true
			} else {
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

// Phase label prefixes. Kept as constants so guess.go and tests stay in sync with
// the descriptions emitted in src/temp.txt (e.g. "CPU Super Core 1:Tp00:M5").
const (
	labelSuperCore       = "CPU Super Core"
	labelPerformanceCore = "CPU Performance Core"
	labelEfficiencyCore  = "CPU Efficiency Core"
)

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

// buildPhases constructs the ordered list of stress phases from live topology data
// and the SKU's pair signature.
//
// 3-level perflevel hierarchy: Super → Performance → Efficiency (e.g. a future
// M-series chip exposing all three tiers as distinct OS perflevels).
//
// 2-level hierarchy: top + bottom labels are driven by layout.PairSignature so the
// same Tp* prefix is named correctly for the SKU. Mapping is:
//   - P+E (M1–M4, A18 across all variants): Performance + Efficiency
//   - S+E (M5 base): Super + Efficiency — the OS may name perflevel0 "Performance"
//     but the family-roster pair signature is authoritative; per the skill,
//     low-stride Tp0? slots on M5 base are Super cores, not Performance cores.
//   - S+P (M5 Pro / M5 Max): Super + Performance — no E-cores on these SKUs
//
// nil/empty perflevels falls back to P+E with zero core counts (legacy behaviour
// for hardware where the perflevel sysctls are unavailable).
//
// The SKU's preferred top-tier QoS class (UserInteractive for Super, UserInitiated
// for Performance) biases the kernel scheduler toward the right tier.
func buildPhases(perfLevels []platform.PerfLevel, layout platform.SKULayout) []phaseSpec {
	if len(perfLevels) == 3 {
		return []phaseSpec{
			{label: labelSuperCore, qos: stress.QoSUserInteractive, cores: perfLevels[0].PhysicalCPU},
			{label: labelPerformanceCore, qos: stress.QoSUserInitiated, cores: perfLevels[1].PhysicalCPU},
			{label: labelEfficiencyCore, qos: stress.QoSBackground, cores: perfLevels[2].PhysicalCPU},
		}
	}

	topLabel, bottomLabel := labelPerformanceCore, labelEfficiencyCore
	topQoS := stress.QoSUserInitiated

	switch layout.PairSignature {
	case platform.PairSignatureSE:
		topLabel = labelSuperCore
		topQoS = stress.QoSUserInteractive
	case platform.PairSignatureSP:
		topLabel, bottomLabel = labelSuperCore, labelPerformanceCore
		topQoS = stress.QoSUserInteractive
	}

	topCores, bottomCores := 0, 0
	if len(perfLevels) == 2 {
		topCores = perfLevels[0].PhysicalCPU
		bottomCores = perfLevels[1].PhysicalCPU
	}

	bottomQoS := stress.QoSBackground
	if layout.PairSignature == platform.PairSignatureSP {
		// S+P has no E-tier; route bottom phase to Performance cores.
		bottomQoS = stress.QoSUserInitiated
	}

	return []phaseSpec{
		{label: topLabel, qos: topQoS, cores: topCores},
		{label: bottomLabel, qos: bottomQoS, cores: bottomCores},
	}
}

// spinCore locks the goroutine to an OS thread, sets the QoS class to bias the OS
// scheduler toward the desired core type, sets a macOS thread-affinity tag to prefer
// a specific hardware thread within that type, then burns CPU until done is closed.
func spinCore(affinityTag int, qosClass int, done <-chan struct{}) {
	runtime.LockOSThread()

	defer runtime.UnlockOSThread()

	// QoS before affinity so placement sees the class.
	stress.SetQoS(qosClass)
	stress.SetAffinityTag(affinityTag)

	// Four independent FMA+sqrt chains saturate all four FP ports.
	a, b, c, d := 1.0001, 1.0003, 1.0007, 1.0013

	for {
		for range 2_500 {
			a = math.Sqrt(math.FMA(a, a, math.Pi))
			b = math.Sqrt(math.FMA(b, b, math.E))
			c = math.Sqrt(math.FMA(c, c, math.Sqrt2))
			d = math.Sqrt(math.FMA(d, d, math.Phi))
		}

		// Overflow guard outside hot loop.
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

	product, _ := platform.GetProduct()
	layout, layoutOK := platform.GetSKULayout()
	perfLevels := platform.GetPerfLevels()
	phases := buildPhases(perfLevels, layout)

	fmt.Printf("Platform : %s\n", family)

	if product.CPU != "" {
		fmt.Printf("Model    : %s (%s)\n", product.Name, product.CPU)
	}

	if layoutOK {
		fmt.Printf("SKU      : %s, %d die(s)\n", layout.PairSignature, layout.Dies)
		fmt.Printf("Expected : %s\n", layoutSummary(layout))
	}

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

	printMapping(family, numCPU, perfLevels, layout, layoutOK, product, results)
}

// layoutSummary returns a one-line human-readable description of the SKU's
// expected core composition, e.g. "8–10P + 4E, 16–20 GPU cores" or
// "5–6S + 10–12P, 16–20 GPU cores". A single value (e.g. "4P") is shown when
// the min and max bounds match. Used in the guess command's preamble.
func layoutSummary(l platform.SKULayout) string {
	parts := make([]string, 0, 4)

	if l.SCoresMax > 0 {
		parts = append(parts, formatRange(l.SCoresMin, l.SCoresMax)+"S")
	}

	if l.PCoresMax > 0 {
		parts = append(parts, formatRange(l.PCoresMin, l.PCoresMax)+"P")
	}

	if l.ECoresMax > 0 {
		parts = append(parts, formatRange(l.ECoresMin, l.ECoresMax)+"E")
	}

	cpu := strings.Join(parts, " + ")

	if l.GPUCoresMax > 0 {
		return fmt.Sprintf("%s, %s GPU cores", cpu, formatRange(l.GPUCoresMin, l.GPUCoresMax))
	}

	return cpu
}

// formatRange returns "n" if min == max, else "min–max" using an en-dash.
func formatRange(minN, maxN int) string {
	if minN == maxN {
		return strconv.Itoa(minN)
	}

	return fmt.Sprintf("%d–%d", minN, maxN)
}

// expectedCores returns the SKU-expected (min, max) core count for a phase label.
// Returns (0, 0) when the phase's core type is absent on this SKU (legitimate
// case for S+E and S+P pair signatures), letting the caller skip range checks.
func expectedCores(l platform.SKULayout, phaseLabel string) (int, int) {
	switch phaseLabel {
	case labelSuperCore:
		return l.SCoresMin, l.SCoresMax
	case labelPerformanceCore:
		return l.PCoresMin, l.PCoresMax
	case labelEfficiencyCore:
		return l.ECoresMin, l.ECoresMax
	default:
		return 0, 0
	}
}

// printPhase emits the comment header, per-series mapping rows, and SKU-aware
// validation footer for one phase of guess output. Extracted from printMapping to
// keep that function's cyclomatic complexity below the project lint threshold.
func printPhase(family string, i int, r phaseResult, keys []string,
	layout platform.SKULayout, layoutOK bool,
) {
	groups := groupBySeries(keys)
	seriesKeys := sortedSeriesKeys(groups)

	if len(seriesKeys) == 0 {
		fmt.Printf("// WARNING: Phase %d (%s) produced no sensor responses above threshold.\n",
			i+1, phaseMidWord(r.spec.label))
		fmt.Println("// Run on an idle machine or increase stress duration.")

		if layoutOK {
			if expMin, expMax := expectedCores(layout, r.spec.label); expMax > 0 {
				fmt.Printf("// SKU expects %s for this phase; verify the chip's actual layout.\n",
					formatRange(expMin, expMax))
			}
		}

		return
	}

	coreIdx := 1

	for _, sk := range seriesKeys {
		coreIdx = printSeries(family, r.spec.label, sk, groups[sk], coreIdx)
	}

	detected := coreIdx - 1
	if layoutOK {
		printDetectionVerdict(layout, r.spec.label, detected)
	}
}

// printSeries emits one series block (header comment + per-core sensor lines) and
// returns the next coreIdx. Pulled out of printPhase to flatten nesting. Each
// line is annotated with the canonical description from src/temp.txt (via
// smc.LookupTempDesc) so the operator can see at a glance whether the guessed
// label matches the existing mapping or proposes a new/conflicting one.
func printSeries(family, label, sk string, sensors []string, coreIdx int) int {
	subGroups := groupByStrideWithinSeries(sensors)

	fmt.Printf("// Series %-6s → %d sensor(s) in %d group(s) → %s %d..%d\n",
		sk, len(sensors), len(subGroups), label, coreIdx, coreIdx+len(subGroups)-1)

	if len(subGroups) == 1 && len(subGroups[0]) > 3 {
		fmt.Printf("// NOTE: %d-sensor group; manual review recommended (check src/temp.txt)\n",
			len(subGroups[0]))
	}

	for _, sg := range subGroups {
		for _, k := range sg {
			line := fmt.Sprintf("%s %d:%s:%s", label, coreIdx, k, family)
			fmt.Println(annotateLine(line, label, k, family, coreIdx))
		}

		coreIdx++
	}

	return coreIdx
}

// annotateLine appends a comment to a temp.txt-style mapping line indicating
// whether the key already has a canonical description in the AppleTemp table:
//
//	✓ when src/temp.txt's description matches the guessed label exactly
//	⚠ when the key is mapped but to a different description (likely mis-label)
//	★ when the key is not yet mapped for this family
//
// The fixed alignment column (45) keeps annotations readable even with
// 16-core SKUs whose lines reach ~36 characters.
func annotateLine(line, label, key, family string, coreIdx int) string {
	const annotationCol = 45

	guessed := fmt.Sprintf("%s %d", label, coreIdx)
	existing, ok := smc.LookupTempDesc(key, family)

	switch {
	case !ok:
		return fmt.Sprintf("%-*s // ★ NEW for %s — not yet in src/temp.txt",
			annotationCol, line, family)
	case existing == guessed:
		return fmt.Sprintf("%-*s // ✓ matches src/temp.txt", annotationCol, line)
	default:
		return fmt.Sprintf("%-*s // ⚠ src/temp.txt has %q",
			annotationCol, line, existing)
	}
}

// printDetectionVerdict compares the count of detected core groups against the
// SKU's expected range and emits a one-line OK / WARNING comment. Skips silently
// when the SKU has no cores of this label's type (the absence is handled by
// warnAbsentTypes after the per-phase loop completes).
func printDetectionVerdict(layout platform.SKULayout, label string, detected int) {
	expMin, expMax := expectedCores(layout, label)
	if expMax == 0 {
		return
	}

	mid := phaseMidWord(label)
	expectedStr := formatRange(expMin, expMax)

	switch {
	case detected < expMin:
		fmt.Printf("// WARNING: detected %d %s group(s) but SKU expects %s — "+
			"some cores may have failed to heat up; rerun on an idle machine.\n",
			detected, mid, expectedStr)
	case detected > expMax:
		fmt.Printf("// WARNING: detected %d %s group(s) but SKU expects %s — "+
			"likely a cluster-aggregate sensor mis-classified as a per-core; review.\n",
			detected, mid, expectedStr)
	default:
		fmt.Printf("// OK: detected %d %s group(s), within SKU expectation %s.\n",
			detected, mid, expectedStr)
	}
}

// warnAbsentTypes emits a warning whenever the resolved per-phase results contain
// core groupings for a type the SKU is not supposed to have (e.g. P-cores reported
// on an M5 base, which is S+E only). Mirrors the validate-temp-mappings skill's
// Phase 4a pair-signature validation.
func warnAbsentTypes(l platform.SKULayout, results []phaseResult) {
	for _, r := range results {
		if len(r.deltas) == 0 {
			continue
		}

		_, expMax := expectedCores(l, r.spec.label)
		if expMax > 0 {
			continue
		}

		// Sensors heated for a core type the SKU lacks — kernel routed elsewhere.
		fmt.Printf("// WARNING: phase %q produced sensors but SKU pair signature %s "+
			"has no cores of this type — relabel before pasting into src/temp.txt.\n",
			r.spec.label, l.PairSignature)
	}
}

// printMapping analyses N-phase delta results and emits a src/temp.txt-style mapping.
// Sensors are classified by dominant phase, then grouped by SMC key series and
// further split by stride/gap analysis to assign correct per-core indices.
//
// When layoutOK is true, the SKU's per-type expected counts (S/P/E) are checked
// against the detected per-phase core groups; deviations are emitted as inline
// warnings so the operator knows to inspect the output before copy-pasting it
// into src/temp.txt.
func printMapping(family string, numCPU int, perfLevels []platform.PerfLevel,
	layout platform.SKULayout, layoutOK bool, product platform.Product, results []phaseResult,
) {
	allKeys := make(map[string]struct{})

	for _, r := range results {
		for k := range r.deltas {
			allKeys[k] = struct{}{}
		}
	}

	phaseKeys := make([][]string, len(results))

	var clusterKeys []string

	for key := range allKeys {
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

	if product.CPU != "" {
		fmt.Printf("// SKU: %s (%s)\n", product.CPU, product.Name)
	}

	if layoutOK {
		fmt.Printf("// Pair signature: %s | dies: %d | expected: %s\n",
			layout.PairSignature, layout.Dies, layoutSummary(layout))
	}

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
		printPhase(family, i, r, phaseKeys[i], layout, layoutOK)
	}

	if layoutOK {
		warnAbsentTypes(layout, results)
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

			line := fmt.Sprintf("// %-6s  %s", k, strings.Join(parts, "  "))
			if existing, ok := smc.LookupTempDesc(k, family); ok {
				fmt.Printf("%-50s ← src/temp.txt: %q\n", line, existing)
			} else {
				fmt.Println(line)
			}
		}
	}

	detected := allDetectedKeys(results, clusterKeys)
	printTempTxtCrosscheck(family, detected)
	printReportsCrosscheck(family, detected)
}

// allDetectedKeys returns the union of detected sensor keys across every phase
// plus the cluster/package keys. Used by the temp.txt and reports/ cross-checks
// to compute matched / novel / silent sets without re-walking the per-phase
// data structures.
func allDetectedKeys(results []phaseResult, clusterKeys []string) map[string]struct{} {
	all := make(map[string]struct{})

	for _, r := range results {
		for k := range r.deltas {
			all[k] = struct{}{}
		}
	}

	for _, k := range clusterKeys {
		all[k] = struct{}{}
	}

	return all
}

// printTempTxtCrosscheck emits a summary of the diff between detected SMC keys
// and the set of keys already mapped in src/temp.txt for the family. The
// summary line reports four cardinalities:
//
//	matched: in temp.txt AND detected this run
//	novel:   detected this run but NOT in temp.txt        → candidates to add
//	silent:  in temp.txt but NOT detected this run        → mapping may be stale,
//	         OR sensor failed to heat under stress
//
// When silent keys exist, the first few are listed with their canonical
// descriptions so the operator can spot-check whether they belong to a
// no-longer-present sensor or simply did not fire in this run.
func printTempTxtCrosscheck(family string, detected map[string]struct{}) {
	mapped := smc.MappedTempKeys(family)
	if len(mapped) == 0 {
		return
	}

	matched := 0

	for k := range detected {
		if _, ok := mapped[k]; ok {
			matched++
		}
	}

	novel := len(detected) - matched
	silent := make([]string, 0)

	for k := range mapped {
		if _, ok := detected[k]; !ok {
			silent = append(silent, k)
		}
	}

	sort.Strings(silent)

	fmt.Println()
	fmt.Printf("// %s\n", strings.Repeat("─", 72))
	fmt.Printf("// Cross-check vs src/temp.txt for %q:\n", family)
	fmt.Printf("//   detected: %d  |  mapped: %d  |  matched: %d  |  novel: %d  |  silent: %d\n",
		len(detected), len(mapped), matched, novel, len(silent))

	if len(silent) == 0 {
		return
	}

	const maxSilent = 25

	shown := min(len(silent), maxSilent)

	fmt.Printf("// Mapped in src/temp.txt but silent in this run (showing %d of %d):\n",
		shown, len(silent))

	for _, k := range silent[:shown] {
		fmt.Printf("//   %-6s  %s\n", k, mapped[k])
	}
}

// printReportsCrosscheck compares the detected keys against the union of keys
// observed in reports/ for the same family. The reports represent ground-truth
// sensor presence captured from real machines, so missing detections often
// reflect runtime conditions (e.g. GPU/SSD sensors that don't heat from a
// CPU stress phase) rather than mapping bugs. Truly novel keys (detected this
// run but absent from every prior dump) are worth investigating — they may be
// new firmware additions or evidence the chip is a sub-variant we haven't
// captured in the reports/ set yet.
func printReportsCrosscheck(family string, detected map[string]struct{}) {
	observed := reports.Keys(family)
	if len(observed) == 0 {
		return
	}

	matched := 0

	for k := range detected {
		if _, ok := observed[k]; ok {
			matched++
		}
	}

	silent := make([]string, 0)

	for k := range observed {
		if _, ok := detected[k]; !ok {
			silent = append(silent, k)
		}
	}

	sort.Strings(silent)

	novel := make([]string, 0)

	for k := range detected {
		if _, ok := observed[k]; !ok {
			novel = append(novel, k)
		}
	}

	sort.Strings(novel)

	fmt.Println()
	fmt.Printf("// %s\n", strings.Repeat("─", 72))
	fmt.Printf("// Cross-check vs reports/ observations for %q:\n", family)
	fmt.Printf("//   detected: %d  |  observed: %d  |  matched: %d  |  novel: %d  |  silent: %d\n",
		len(detected), len(observed), matched, len(novel), len(silent))

	if len(novel) > 0 {
		const maxNovel = 20

		shown := min(len(novel), maxNovel)

		fmt.Printf("// Detected but never seen in any %s report (showing %d of %d):\n",
			family, shown, len(novel))

		for _, k := range novel[:shown] {
			fmt.Printf("//   %s\n", k)
		}
	}
}
