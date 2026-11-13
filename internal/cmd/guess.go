// SPDX-FileCopyrightText: Copyright (C) 2026  Dinko Korunic
// SPDX-License-Identifier: GPL-3.0-only

//go:build darwin

package cmd

import (
	"cmp"
	"fmt"
	"maps"
	"math"
	"os"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/dkorunic/iSMC/internal/platform"
	"github.com/dkorunic/iSMC/internal/reports"
	"github.com/dkorunic/iSMC/internal/stress"
	"github.com/dkorunic/iSMC/smc"
	"github.com/spf13/cobra"
)

const (
	guessStressDuration = 8 * time.Second
	guessSampleCount    = 3
	guessSampleInterval = 500 * time.Millisecond

	// Thermal steady-state detection: the signed mean change between
	// consecutive snapshots (~3 s apart) must be below the epsilon. Signed, not
	// absolute: per-key noise cancels across ~100 keys (sp78 ~0.25 °C/key →
	// ~0.03 °C), while a cooling trend does not. An absolute mean sits at the
	// noise level, so on low-noise flt sensors (M6) a chip still cooling by
	// ~4 °C/min passed as settled and zeroed the following E phase.
	guessSettleEpsilon     = float32(0.05)
	guessSettlePollPause   = 2 * time.Second
	guessSettleInitialWait = 20 * time.Second
	guessSettleCooldownMax = 90 * time.Second
	// 25 °C sits in the gap between disconnected probes (≤12) and real idle sensors (≥26).
	guessTempMin = float32(25.0)
	guessTempMax = float32(150.0)

	// ≈5 σ above the noise of a delta between two 3-sample averages (sp78
	// per-read noise ~0.25 °C). Kept low so the small self-heating deltas of
	// E-cores under a background-QoS sweep are retained; classification
	// quality is enforced by median-normalized group scoring, not by this
	// floor.
	guessOutputThreshold = float32(1.0)

	// Sensor is cluster-level if min/max phase delta ≥ (1 − this).
	guessClusterRatio = float32(0.20)

	// A phase competes for a core group only if the group's summed delta there
	// is at least this share of its largest phase delta. Laptop E-cores
	// self-heat about half as much as the P-phase warms them, so they pass; a
	// desktop's 1 °C noise on one key of a P-core (5% of its P-phase rise on an
	// M1 Ultra) does not.
	guessMinPhaseShare = float32(0.25)
)

var guessCmd = &cobra.Command{
	Use:   "guess",
	Short: "Map SMC temperature sensors to CPU cores by thermal correlation",
	Long: `guess stresses one CPU tier per phase — two or three phases depending on the
chip's pair signature — and correlates the resulting temperature rise in
T-prefixed SMC sensors to produce a sensor mapping list in the format used by
src/temp.txt. Each phase spins exactly the target tier's logical-CPU count so
the scheduler keeps the load on that tier instead of spilling onto the other.

Phase labels and QoS hints are driven by the SKU's pair signature (from the
validate-temp-mappings family roster):
  - P+E (M1–M4 family, A18 Pro):   Performance + Efficiency
  - S+E (M5 base):                 Super + Efficiency
  - S+P (M5 Pro / M5 Max):         Super + Performance
  - 3-level perflevel hierarchy:   Super + Performance + Efficiency

QOS_CLASS_USER_INTERACTIVE biases the kernel toward Super cores, USER_INITIATED
toward Performance cores, and BACKGROUND toward Efficiency cores.

Within each phase, per-core labels are derived from SMC key naming patterns:
keys sharing the same structure form a series (indices decode positionally over
the 0-9A-Za-z alphabet), gap analysis within each series groups sensors that
belong to the same physical core, and only per-core families (Tp*/Te* on Apple
Silicon) are labelled as cores — other responding sensors are listed for
reference. The detected per-type counts are cross-checked against the SKU's
expected layout (e.g. M5 Pro: 5–6 Super + 10–12 Performance), and deviations
are flagged inline so the operator knows which lines need manual review before
pasting into src/temp.txt.

Baselines wait for thermal steady state, so total runtime varies (roughly one
to three minutes). Run on an otherwise-idle machine for best results.

On desktop-class machines with large coolers (Mac Studio, Mac Pro), E-core
self-heating under a background-QoS sweep is below the SMC noise floor, so
E-cores show up only as the weakest responders of the Performance sweep. When
one tier then exceeds the SKU's core count and another falls short, the weakest
surplus groups are moved to the short tier and a NOTE says so; verify those
lines. Annotations: ✓ matches src/temp.txt, ~ same tier but another core
number, ✗ a different tier, ⚠ a non-core label, ★ unmapped.

--gpu adds a final phase that loads the GPU with a compute-bound Metal kernel
and lists the sensors that respond, comparing each key's GPU rise with its
largest CPU-phase rise. It tells GPU-tier keys from package-wide ones and flags
src/temp.txt labels that disagree; it cannot number GPU cores, because the
kernel loads every core evenly.`,
	Run: runGuess,
}

// guessGPU enables the Metal GPU phase (--gpu).
var guessGPU bool

func init() {
	guessCmd.Flags().BoolVar(&guessGPU, "gpu", false, "add a GPU load phase to classify GPU-tier sensors")
	rootCmd.AddCommand(guessCmd)
}

// tempSampler samples SMC temperature sensors over one long-lived connection,
// resolving the T-prefixed keys once so each snapshot skips re-walking the full
// namespace — the dominant cost of a guess run.
type tempSampler struct {
	keys []string
	conn uint
}

// newTempSampler resolves conn's T-prefixed keys once for the sampler's lifetime.
func newTempSampler(conn uint) *tempSampler {
	return &tempSampler{conn: conn, keys: smc.EnumerateKeysPrefix(conn, "T")}
}

// rawTemps returns key → °C for the sampler's keys reporting a plausible value.
func (s *tempSampler) rawTemps() map[string]float32 {
	out := make(map[string]float32)

	for _, key := range s.keys {
		rk, ok := smc.ReadRawKey(s.conn, key)
		if !ok {
			continue
		}

		v, ok := smc.RawKeyToFloat32(rk)
		if !ok || v < guessTempMin || v > guessTempMax {
			continue
		}

		out[key] = v
	}

	return out
}

// avgRawTemps returns per-key averages over n samples taken sampleInterval apart.
func (s *tempSampler) avgRawTemps(n int, interval time.Duration) map[string]float32 {
	sums := make(map[string]float64)
	counts := make(map[string]int)

	for i := range n {
		if i > 0 {
			time.Sleep(interval)
		}

		for k, v := range s.rawTemps() {
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

// meanDelta returns the signed mean per-key change from snapshot a to b over
// their common keys (negative while cooling), or 0 when no keys are shared.
func meanDelta(a, b map[string]float32) float32 {
	var sum float64

	n := 0

	for k, av := range a {
		if bv, ok := b[k]; ok {
			sum += float64(bv - av)
			n++
		}
	}

	if n == 0 {
		return 0
	}

	return float32(sum / float64(n))
}

// settledTemps samples until consecutive averages differ by less than
// guessSettleEpsilon (thermal steady state) or maxWait elapses. A fixed cooldown
// is not enough: a baseline sampled from a still-cooling chip biases the next
// phase's deltas — warm starts previously collapsed whole phases to zero.
func (s *tempSampler) settledTemps(maxWait time.Duration) map[string]float32 {
	prev := s.avgRawTemps(2, guessSampleInterval)
	deadline := time.Now().Add(maxWait)

	for time.Now().Before(deadline) {
		time.Sleep(guessSettlePollPause)

		cur := s.avgRawTemps(2, guessSampleInterval)
		if math.Abs(float64(meanDelta(prev, cur))) < float64(guessSettleEpsilon) {
			return cur
		}

		prev = cur
	}

	return prev
}

// reportDeltas returns deltaTemps(base, hot) and prints the responder count.
// With no responders it also prints the raw change range, which tells a phase
// that barely warmed (small positive max) from one measured against a
// still-cooling baseline (negative max).
func reportDeltas(base, hot map[string]float32) map[string]float32 {
	deltas := deltaTemps(base, hot)

	fmt.Printf("  →  %d sensor(s) responded\n", len(deltas))

	if len(deltas) == 0 {
		if lo, hi, ok := deltaRange(base, hot); ok {
			fmt.Printf("     raw change %+.2f°C (%s) .. %+.2f°C (%s), mean %+.2f°C\n",
				lo.d, lo.key, hi.d, hi.key, meanDelta(base, hot))
		}
	}

	return deltas
}

// keyDelta is one sensor's change between two snapshots.
type keyDelta struct {
	key string
	d   float32
}

// deltaRange returns the smallest and largest per-key change from base to hot,
// ties broken by key for stable output; ok is false when no keys are shared.
func deltaRange(base, hot map[string]float32) (keyDelta, keyDelta, bool) {
	var lo, hi keyDelta

	ok := false

	for k, b := range base {
		h, found := hot[k]
		if !found {
			continue
		}

		d := keyDelta{key: k, d: h - b}

		if !ok || d.d < lo.d || (d.d == lo.d && k < lo.key) {
			lo = d
		}

		if !ok || d.d > hi.d || (d.d == hi.d && k < hi.key) {
			hi = d
		}

		ok = true
	}

	return lo, hi, ok
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

// charValue returns the positional value of an SMC index char in the 0-9<A-Z<a-z
// alphabet, or -1 if outside it.
func charValue(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'A' && c <= 'Z':
		return int(c-'A') + 10
	case c >= 'a' && c <= 'z':
		return int(c-'a') + 36
	default:
		return -1
	}
}

// indexBase is the radix of the positional SMC index alphabet (0-9A-Za-z).
const indexBase = 62

// splitKey splits an SMC key into its series identity (structural characters,
// index positions replaced by '*') and its numeric index.
//
// Apple Silicon per-core families use a lowercase family letter followed by a
// 2-character positional base-62 index: "Tp0y" → 60, "Tp0z" → 61, "Tp10" → 62
// are adjacent, and "Tp29" → 133 neighbours "Tp2A" → 134. Classic keys
// (uppercase families on any platform, and every key on Intel, where the
// "Tc0a" shape means index digit + probe letter) use a digits-only decimal
// index with all letters kept as series structure.
func splitKey(key string, appleSilicon bool) (string, int) {
	if appleSilicon && len(key) == 4 && key[0] == 'T' &&
		key[1] >= 'a' && key[1] <= 'z' &&
		charValue(key[2]) >= 0 && charValue(key[3]) >= 0 {
		return key[:2] + "**", charValue(key[2])*indexBase + charValue(key[3])
	}

	b := []byte(key)

	var digits []byte

	for i, c := range b {
		if c >= '0' && c <= '9' {
			digits = append(digits, c)
			b[i] = '*'
		}
	}

	// Empty digits leaves v at 0, matching keys with no index (e.g. TCDX).
	v, _ := strconv.Atoi(string(digits))

	return string(b), v
}

// groupBySeries groups SMC sensor keys by series identity. Within each group
// the keys are sorted ascending by index so that sorted position 0 maps to
// Core 1, position 1 maps to Core 2, etc.
func groupBySeries(keys []string, appleSilicon bool) map[string][]string {
	groups := make(map[string][]string)

	for _, k := range keys {
		sk, _ := splitKey(k, appleSilicon)
		groups[sk] = append(groups[sk], k)
	}

	for sk := range groups {
		slices.SortFunc(groups[sk], func(a, b string) int {
			_, av := splitKey(a, appleSilicon)
			_, bv := splitKey(b, appleSilicon)

			return cmp.Compare(av, bv)
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

	slices.Sort(keys)

	return keys
}

// groupCores splits a series' sensor list into per-core groups.
//
// The list is first split into contiguous runs wherever the index gap between
// consecutive keys exceeds 1 (probes of one core are index-adjacent; distinct
// cores are separated by at least one unpopulated slot on every observed
// platform except M4 base, handled below). Each run then becomes core groups:
//
//   - Single-key convention (M5 family, pair signatures S+E / S+P): every key
//     is one core; runs are split into singletons.
//   - Triplet convention (M1–M4, A18): a run is one core's probe group, except
//     contiguous runs holding several triplets back-to-back (M4 base packs
//     Tp0U..Tp0f with no gaps) which are chunked into threes.
func groupCores(sensors []string, appleSilicon, singleKeyConvention bool) [][]string {
	sorted := slices.SortedFunc(slices.Values(sensors), func(a, b string) int {
		_, av := splitKey(a, appleSilicon)
		_, bv := splitKey(b, appleSilicon)

		return cmp.Compare(av, bv)
	})

	var runs [][]string

	runStart := 0
	prev := 0

	for i, s := range sorted {
		_, idx := splitKey(s, appleSilicon)
		if i > 0 && idx-prev > 1 {
			runs = append(runs, sorted[runStart:i])
			runStart = i
		}

		prev = idx
	}

	if runStart < len(sorted) {
		runs = append(runs, sorted[runStart:])
	}

	var groups [][]string

	for _, run := range runs {
		switch {
		case singleKeyConvention:
			for _, s := range run {
				groups = append(groups, []string{s})
			}
		case len(run) > 3 && len(run)%3 == 0:
			for i := 0; i < len(run); i += 3 {
				groups = append(groups, run[i:i+3])
			}
		default:
			groups = append(groups, run)
		}
	}

	return groups
}

// coreEligibleSeries reports whether sensors in the given series may be
// labelled as CPU cores. On Apple Silicon the Tp (Super/Performance) family
// always qualifies; Te qualifies only when tePerCore is set (M3/M4/A18 expose
// per-core E sensors there, while on M1/M2 the Te* keys are die and cluster
// aggregates). On Intel the classic TC*C/TC*c per-core keys and the T2's
// Tc-quad probes qualify. Everything else (SSD, GPU, heatsink, aggregates,
// proximity, virtual sensors) responds to CPU stress through package heating
// but is not a core sensor.
func coreEligibleSeries(series string, appleSilicon, tePerCore bool) bool {
	if appleSilicon {
		return series == "Tp**" || (tePerCore && series == "Te**")
	}

	switch series {
	case "Tc*a", "Tc*b", "Tc*x", "Tc*z", "TC*C", "TC*c":
		return true
	default:
		return false
	}
}

// groupPhaseDelta returns the summed deltas of a group's keys in one phase's
// delta map; keys below the output threshold are absent and contribute 0.
func groupPhaseDelta(group []string, deltas map[string]float32) float32 {
	var sum float32

	for _, k := range group {
		sum += deltas[k]
	}

	return sum
}

// medianPositive returns the median of the positive values in vals, or 0 when
// none are positive.
func medianPositive(vals []float32) float32 {
	positive := make([]float32, 0, len(vals))

	for _, v := range vals {
		if v > 0 {
			positive = append(positive, v)
		}
	}

	if len(positive) == 0 {
		return 0
	}

	slices.Sort(positive)

	mid := len(positive) / 2
	if len(positive)%2 == 1 {
		return positive[mid]
	}

	return (positive[mid-1] + positive[mid]) / 2
}

// classifyCoreGroups assigns each per-core group to the phase whose
// median-normalized response is strongest, or to the cluster bucket when the
// top two normalized scores are within guessClusterRatio of each other.
//
// Raw deltas cannot be compared across phases: a P-tier sweep heats the whole
// die, so an E-core sensor's absolute delta under P-stress often exceeds its
// self-heating delta under E-stress. Dividing each group's per-phase delta by
// that phase's median positive group delta measures how strongly the group
// responded relative to its peers, which survives the coupling.
func classifyCoreGroups(groups [][]string, results []phaseResult) ([][][]string, [][]string) {
	sums := make([][]float32, len(results))

	for i, r := range results {
		sums[i] = make([]float32, len(groups))

		for g, group := range groups {
			sums[i][g] = groupPhaseDelta(group, r.deltas)
		}
	}

	medians := make([]float32, len(results))
	for i := range results {
		medians[i] = medianPositive(sums[i])
	}

	perPhase := make([][][]string, len(results))

	var clusters [][]string

	for g, group := range groups {
		best, second := -1, float32(0)

		var bestScore, peak float32

		for i := range results {
			peak = max(peak, sums[i][g])
		}

		for i := range results {
			// A phase that barely warmed this group is noise, not a competing
			// response: normalizing it by a median of other noise scores ~1.0.
			if medians[i] <= 0 || sums[i][g] < guessMinPhaseShare*peak {
				continue
			}

			score := sums[i][g] / medians[i]
			if score > bestScore {
				second = bestScore
				bestScore = score
				best = i
			} else if score > second {
				second = score
			}
		}

		switch {
		case best < 0:
			// No phase produced a response; drop silently (cannot happen for
			// groups built from detected keys, kept for safety).
		case second > 0 && second/bestScore >= 1-guessClusterRatio:
			clusters = append(clusters, group)
		default:
			perPhase[best] = append(perPhase[best], group)
		}
	}

	return perPhase, clusters
}

// rebalanceToLayout moves surplus groups from the one phase above its SKU
// maximum into the one phase below its SKU minimum, weakest first by the
// source phase's mean per-key delta, and returns the moved groups (nil when
// nothing moved) with the source and destination phase indices.
//
// On desktop coolers E-core self-heating under a background sweep sits below
// the noise floor, so E-cores only show up as weak responders in the P sweep
// (M1 Ultra: E groups ~3.7–4.2 °C per key against ~7.6–9.6 °C for P-cores).
// The SKU's core counts say how many of them are surplus. Moved groups keep
// their original order (groups) so core numbering stays positional.
// ponytail: only the unambiguous one-over/one-under case; 3-tier SKUs where
// several tiers miss would need per-tier affinity scores.
func rebalanceToLayout(perPhase [][][]string, results []phaseResult, layout platform.SKULayout,
	groups [][]string,
) ([][]string, int, int) {
	src, dst := -1, -1

	for i, r := range results {
		expMin, expMax := expectedCores(layout, r.spec.label)

		switch {
		case expMax > 0 && len(perPhase[i]) > expMax:
			if src >= 0 {
				return nil, 0, 0
			}

			src = i
		// A phase with no responders failed to load its tier; filling it
		// would invent cores (M6 E phase: 0 responders, 5 Te keys moved in).
		case len(perPhase[i]) < expMin && len(r.deltas) > 0:
			if dst >= 0 {
				return nil, 0, 0
			}

			dst = i
		}
	}

	if src < 0 || dst < 0 {
		return nil, 0, 0
	}

	_, srcMax := expectedCores(layout, results[src].spec.label)
	dstMin, _ := expectedCores(layout, results[dst].spec.label)
	n := min(len(perPhase[src])-srcMax, dstMin-len(perPhase[dst])) //nolint:gosec // G602: src/dst index results, and perPhase has one entry per result

	weakest := slices.Clone(perPhase[src])
	slices.SortStableFunc(weakest, func(a, b []string) int {
		return cmp.Compare(groupMeanDelta(a, results[src].deltas), groupMeanDelta(b, results[src].deltas))
	})

	moved := make(map[string]bool, n)
	for _, g := range weakest[:n] {
		moved[g[0]] = true
	}

	perPhase[src] = slices.DeleteFunc(perPhase[src], func(g []string) bool { return moved[g[0]] })
	perPhase[dst] = append(perPhase[dst], weakest[:n]...) //nolint:gosec // G602: src/dst index results, and perPhase has one entry per result

	order := make(map[string]int, len(groups))
	for i, g := range groups {
		order[g[0]] = i
	}

	slices.SortFunc(perPhase[dst], func(a, b []string) int { //nolint:gosec // G602: see above
		return cmp.Compare(order[a[0]], order[b[0]])
	})

	return weakest[:n], src, dst
}

// groupMeanDelta returns the mean delta of a group's keys that responded in
// one phase. Unlike the sum it does not penalise a core whose third key
// stayed under guessOutputThreshold (M1 Ultra: Tp04/Tp05 without Tp06).
func groupMeanDelta(group []string, deltas map[string]float32) float32 {
	var sum float32

	n := 0

	for _, k := range group {
		if d, ok := deltas[k]; ok {
			sum += d
			n++
		}
	}

	if n == 0 {
		return 0
	}

	return sum / float32(n)
}

// Phase label prefixes; must match descriptions emitted in src/temp.txt.
const (
	labelSuperCore       = "CPU Super Core"
	labelPerformanceCore = "CPU Performance Core"
	labelEfficiencyCore  = "CPU Efficiency Core"
)

// phaseSpec describes one stress phase: its label prefix, QoS class, expected
// physical core count, and the number of spinner threads to launch (the target
// tier's logical CPU count, so the scheduler keeps the load on that tier
// instead of spilling onto the other one).
type phaseSpec struct {
	label   string
	qos     int
	cores   int // 0 if unknown.
	threads int // 0 → fall back to all logical CPUs.
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
			{
				label: labelSuperCore, qos: stress.QoSUserInteractive,
				cores: perfLevels[0].PhysicalCPU, threads: perfLevels[0].LogicalCPU,
			},
			{
				label: labelPerformanceCore, qos: stress.QoSUserInitiated,
				cores: perfLevels[1].PhysicalCPU, threads: perfLevels[1].LogicalCPU,
			},
			{
				label: labelEfficiencyCore, qos: stress.QoSBackground,
				cores: perfLevels[2].PhysicalCPU, threads: perfLevels[2].LogicalCPU,
			},
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
	topThreads, bottomThreads := 0, 0

	if len(perfLevels) == 2 {
		topCores = perfLevels[0].PhysicalCPU
		bottomCores = perfLevels[1].PhysicalCPU
		topThreads = perfLevels[0].LogicalCPU
		bottomThreads = perfLevels[1].LogicalCPU
	}

	bottomQoS := stress.QoSBackground
	if layout.PairSignature == platform.PairSignatureSP {
		// S+P has no E-tier; route bottom phase to Performance cores.
		bottomQoS = stress.QoSUserInitiated
	}

	return []phaseSpec{
		{label: topLabel, qos: topQoS, cores: topCores, threads: topThreads},
		{label: bottomLabel, qos: bottomQoS, cores: bottomCores, threads: bottomThreads},
	}
}

// spinCore locks the goroutine to an OS thread, sets the QoS class to bias the OS
// scheduler toward the desired core type, then burns CPU until done is closed.
// THREAD_AFFINITY_POLICY is not implemented on Apple Silicon, so QoS class plus
// per-tier thread counts are the only placement controls available.
func spinCore(qosClass int, done <-chan struct{}) {
	runtime.LockOSThread()

	defer runtime.UnlockOSThread()

	stress.SetQoS(qosClass)

	// Four FMA+sqrt chains saturate all FP ports.
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

// runStressPhase spins threads goroutines at qosClass for guessStressDuration,
// samples at peak load, and returns the per-sensor delta over baseline.
func runStressPhase(s *tempSampler, threads, qosClass int, baseline map[string]float32) map[string]float32 {
	done := make(chan struct{})

	for range threads {
		go spinCore(qosClass, done)
	}

	time.Sleep(guessStressDuration)

	hot := s.avgRawTemps(guessSampleCount, guessSampleInterval)

	close(done)

	return reportDeltas(baseline, hot)
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

	switch {
	case product.Name != "":
		fmt.Printf("Model    : %s (%s)\n", product.Name, product.CPU)
	case product.CPU != "":
		fmt.Printf("Model    : %s\n", product.CPU)
	}

	if layoutOK {
		fmt.Printf("SKU      : %s, %d die(s)\n", layout.PairSignature, layout.Dies)
		fmt.Printf("Expected : %s\n", layoutSummary(layout))
	}

	fmt.Printf("CPUs     : %d logical\n", numCPU)
	fmt.Printf("Per-phase: %v stress  (×%d phases, one spinner per target-tier CPU)\n\n",
		guessStressDuration, len(phases))

	if len(perfLevels) > 0 {
		fmt.Printf("Topology : %d perf level(s)\n", len(perfLevels))

		for i, pl := range perfLevels {
			fmt.Printf("           perflevel%d: %d phys-CPU %q\n", i, pl.PhysicalCPU, pl.Name)
		}

		fmt.Println()
	}

	// One connection and key enumeration for the whole run.
	conn, err := smc.Open()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)

		return
	}
	defer smc.Close(conn)

	sampler := newTempSampler(conn)

	fmt.Print("Sampling baseline (waiting for thermal steady state)... ")

	baseline := sampler.settledTemps(guessSettleInitialWait)

	fmt.Printf("%d sensors visible.\n\n", len(baseline))

	results := make([]phaseResult, 0, len(phases))

	for i, phase := range phases {
		threads := phase.threads
		if threads <= 0 {
			threads = numCPU
		}

		fmt.Printf("── Phase %d: %s sweep (%s QoS) ──\n",
			i+1, phaseMidWord(phase.label), qosName(phase.qos))
		fmt.Printf("  Stressing %d thread(s)...", threads)

		deltas := runStressPhase(sampler, threads, phase.qos, baseline)
		results = append(results, phaseResult{spec: phase, deltas: deltas})

		if i < len(phases)-1 {
			fmt.Printf("\n  Inter-phase cooldown (up to %v, until temperatures settle)...\n\n",
				guessSettleCooldownMax)

			baseline = sampler.settledTemps(guessSettleCooldownMax)
		}
	}

	var gpuDeltas map[string]float32

	if guessGPU {
		fmt.Printf("\n  Inter-phase cooldown (up to %v, until temperatures settle)...\n\n",
			guessSettleCooldownMax)

		baseline = sampler.settledTemps(guessSettleCooldownMax)

		fmt.Printf("── Phase %d: GPU sweep (Metal compute) ──\n", len(phases)+1)
		fmt.Print("  Loading GPU...")

		gpuDeltas = runGPUPhase(sampler, baseline)
	}

	printMapping(family, numCPU, perfLevels, layout, layoutOK, product, results, baseline)

	if gpuDeltas != nil {
		printGPUPhase(family, product.CPU, gpuDeltas, baseline, results)
	}
}

// runGPUPhase loads the GPU for guessStressDuration, samples at peak load and
// returns the per-sensor delta over baseline, or nil when Metal is unavailable.
func runGPUPhase(s *tempSampler, baseline map[string]float32) map[string]float32 {
	stop, err := stress.StartGPU()
	if err != nil {
		fmt.Printf("  skipped: %v\n", err)

		return nil
	}

	time.Sleep(guessStressDuration)

	hot := s.avgRawTemps(guessSampleCount, guessSampleInterval)

	stop()

	return reportDeltas(baseline, hot)
}

// A key is GPU-tier when its GPU-phase rise is at least gpuDominanceRatio × its
// largest CPU-phase rise AND at least gpuTierFloor × the run's largest GPU rise.
// The ratio alone is not enough: keys the CPU phases never warm (memory, RF and
// power delivery near the GPU) pass it with a 1–3 °C GPU rise. On an M1 Ultra
// run the floor split cleanly: Tg* and GPU-side TC* groups rose 5.8–11 °C,
// everything else at most 4.9 °C.
// ponytail: calibrated on one M1 Ultra run; retune with runs from other SKUs.
const (
	gpuDominanceRatio = float32(2.0)
	gpuTierFloor      = float32(0.5)
)

// isGPUTier reports whether a key's GPU rise marks it as sitting on the GPU.
func isGPUTier(gpuDelta, maxCPUDelta, maxGPUDelta float32) bool {
	return gpuDelta >= gpuDominanceRatio*maxCPUDelta && gpuDelta >= gpuTierFloor*maxGPUDelta
}

// printGPUPhase lists GPU-tier and GPU-labelled responders by GPU rise and flags
// src/temp.txt labels that disagree: a GPU-tier key whose label lacks "GPU", a
// "GPU" key that is not GPU-tier, and mapped "GPU" keys this machine reads that
// did not respond at all. Other responders are only counted.
func printGPUPhase(family, sku string, gpuDeltas, visible map[string]float32, results []phaseResult) {
	keys := make([]string, 0, len(gpuDeltas))

	var maxGPU float32

	for k, d := range gpuDeltas {
		keys = append(keys, k)
		maxGPU = max(maxGPU, d)
	}

	slices.SortFunc(keys, func(a, b string) int {
		return cmp.Or(cmp.Compare(gpuDeltas[b], gpuDeltas[a]), cmp.Compare(a, b))
	})

	fmt.Println()
	fmt.Printf("// %s\n", strings.Repeat("─", 72))
	fmt.Printf("// GPU phase (reference only; GPU-tier = rise ≥ %.0f× max CPU rise and ≥ %.1f°C):\n",
		gpuDominanceRatio, gpuTierFloor*maxGPU)

	others := 0

	for _, k := range keys {
		var maxCPU float32
		for _, r := range results {
			maxCPU = max(maxCPU, r.deltas[k])
		}

		tier := isGPUTier(gpuDeltas[k], maxCPU, maxGPU)
		desc, mapped := smc.LookupTempDesc(k, family, sku)
		gpuLabel := strings.Contains(desc, "GPU")

		if !tier && !gpuLabel {
			others++

			continue
		}

		class, note := "GPU-tier", ""

		switch {
		case !tier:
			class, note = "shared  ", "  ⚠ GPU label but not GPU-tier"
		case !mapped:
			desc = "(unmapped)"
		case !gpuLabel:
			note = "  ⚠ GPU-tier but label is not GPU"
		}

		if mapped {
			desc = strconv.Quote(desc)
		}

		fmt.Printf("//   %-6s GPU +%4.1f°C  CPU +%4.1f°C  %s  %s%s\n",
			k, gpuDeltas[k], maxCPU, class, desc, note)
	}

	fmt.Printf("// %d other responder(s) warmed through the package (not listed).\n", others)

	silent := make([]string, 0)

	for k, desc := range smc.MappedTempKeys(family, sku) {
		if _, seen := visible[k]; seen && gpuDeltas[k] == 0 && strings.Contains(desc, "GPU") {
			silent = append(silent, k)
		}
	}

	if len(silent) == 0 {
		return
	}

	slices.Sort(silent)

	fmt.Printf("// GPU-labelled in src/temp.txt but silent under GPU load (%d): %s\n",
		len(silent), strings.Join(silent, " "))
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

// printPhase emits the comment header, per-core mapping rows, and SKU-aware
// validation footer for one phase of guess output. Extracted from printMapping to
// keep that function's cyclomatic complexity below the project lint threshold.
//
// groups holds the per-core groups classifyCoreGroups assigned to this phase;
// every other responding sensor is listed in a reference-only section so
// package-heated SSD/GPU/heatsink/aggregate sensors no longer masquerade as
// cores.
func printPhase(family, sku string, i int, r phaseResult, groups [][]string,
	layout platform.SKULayout, layoutOK, appleSilicon, tePerCore bool,
) {
	if len(groups) == 0 {
		fmt.Printf("// WARNING: Phase %d (%s) produced no per-core sensor responses above threshold.\n",
			i+1, phaseMidWord(r.spec.label))
		fmt.Println("// Run on an idle machine or increase stress duration.")

		if expMin, expMax := expectedCores(layout, r.spec.label); layoutOK && expMax > 0 {
			fmt.Printf("// SKU expects %s for this phase; verify the chip's actual layout.\n",
				formatRange(expMin, expMax))
		}
	} else {
		printCoreGroups(family, sku, i, r.spec.label, groups)

		if layoutOK {
			printDetectionVerdict(layout, r.spec.label, len(groups))
		}
	}

	otherKeys := make([]string, 0, len(r.deltas))

	for k := range r.deltas {
		if sk, _ := splitKey(k, appleSilicon); !coreEligibleSeries(sk, appleSilicon, tePerCore) {
			otherKeys = append(otherKeys, k)
		}
	}

	printNonCoreResponders(family, sku, r, otherKeys)
}

// printCoreGroups emits the temp.txt-style mapping lines for one phase's
// assigned core groups, numbering cores sequentially and annotating each line
// against the canonical src/temp.txt description.
func printCoreGroups(family, sku string, i int, label string, groups [][]string) {
	fmt.Printf("// Phase %d (%s): %d core group(s)\n", i+1, phaseMidWord(label), len(groups))

	for coreIdx, g := range groups {
		if len(g) > 3 {
			fmt.Printf("// NOTE: %d-sensor group; manual review recommended (check src/temp.txt)\n",
				len(g))
		}

		for _, k := range g {
			line := fmt.Sprintf("%s %d:%s:%s", label, coreIdx+1, k, family)
			fmt.Println(annotateLine(line, label, k, family, sku, coreIdx+1))
		}
	}
}

// printNonCoreResponders lists sensors that responded to a phase's stress but
// belong to non-core series (SSD, GPU, heatsink, die aggregates, proximity,
// virtual). They heat through the package; labelling them as CPU cores was the
// main source of phantom cores in earlier guess output.
func printNonCoreResponders(family, sku string, r phaseResult, keys []string) {
	if len(keys) == 0 {
		return
	}

	slices.Sort(keys)

	fmt.Printf("// Non-core responders in the %s phase (reference only, do not paste):\n",
		phaseMidWord(r.spec.label))

	for _, k := range keys {
		desc := "(unmapped)"
		if existing, ok := smc.LookupTempDesc(k, family, sku); ok {
			desc = fmt.Sprintf("%q", existing)
		}

		fmt.Printf("//   %-6s +%.1f°C  %s\n", k, r.deltas[k], desc)
	}
}

// annotateLine appends a comment to a temp.txt-style mapping line indicating
// whether the key already has a canonical description in the AppleTemp table:
//
//	✓ when src/temp.txt's description matches the guessed label exactly
//	~ when it is the same core tier under a different core number
//	✗ when it is a core of a different tier (a real classification conflict)
//	⚠ when the key is mapped to some other, non-core description
//	★ when the key is not yet mapped for this family
//
// The fixed alignment column (45) keeps annotations readable even with
// 16-core SKUs whose lines reach ~36 characters.
func annotateLine(line, label, key, family, sku string, coreIdx int) string {
	const annotationCol = 45

	guessed := fmt.Sprintf("%s %d", label, coreIdx)
	existing, ok := smc.LookupTempDesc(key, family, sku)

	switch {
	case !ok:
		return fmt.Sprintf("%-*s // ★ NEW for %s — not yet in src/temp.txt",
			annotationCol, line, family)
	case existing == guessed:
		return fmt.Sprintf("%-*s // ✓ matches src/temp.txt", annotationCol, line)
	case strings.HasPrefix(existing, label+" "):
		return fmt.Sprintf("%-*s // ~ same tier, src/temp.txt numbers it %q",
			annotationCol, line, existing)
	case strings.Contains(existing, " Core "):
		return fmt.Sprintf("%-*s // ✗ tier mismatch: src/temp.txt has %q",
			annotationCol, line, existing)
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

// warnAbsentTypes emits a warning whenever core groups were assigned to a phase
// whose core type the SKU is not supposed to have (e.g. P-cores reported on an
// M5 base, which is S+E only). Mirrors the validate-temp-mappings skill's
// Phase 4a pair-signature validation.
func warnAbsentTypes(l platform.SKULayout, results []phaseResult, perPhase [][][]string) {
	for i, r := range results {
		if len(perPhase[i]) == 0 {
			continue
		}

		_, expMax := expectedCores(l, r.spec.label)
		if expMax > 0 {
			continue
		}

		// Core groups resolved for a core type the SKU lacks; kernel routed elsewhere.
		fmt.Printf("// WARNING: phase %q produced core groups but SKU pair signature %s "+
			"has no cores of this type — relabel before pasting into src/temp.txt.\n",
			r.spec.label, l.PairSignature)
	}
}

// singleKeyLayout reports whether the SKU exposes one sensor key per core (M5
// and later: S+E, S+P, S+P+E) rather than M1-M4 style probe triplets. On M6 the
// triplet rule would merge Te0e-Te0i into one core and Tp0w-Tp0z into another.
func singleKeyLayout(l platform.SKULayout) bool {
	switch l.PairSignature {
	case platform.PairSignatureSE, platform.PairSignatureSP, platform.PairSignatureSPE:
		return true
	default:
		return false
	}
}

// printMapping analyses N-phase delta results and emits a src/temp.txt-style mapping.
// Core-eligible sensors are grouped by series and per-core index structure over
// the union of all phases, then each group is assigned to the phase with the
// strongest median-normalized response (see classifyCoreGroups). Non-core
// sensors are listed per phase for reference only.
//
// When layoutOK is true, the SKU's per-type expected counts (S/P/E) are checked
// against the detected per-phase core groups; deviations are emitted as inline
// warnings so the operator knows to inspect the output before copy-pasting it
// into src/temp.txt.
func printMapping(family string, numCPU int, perfLevels []platform.PerfLevel,
	layout platform.SKULayout, layoutOK bool, product platform.Product, results []phaseResult,
	visible map[string]float32,
) {
	appleSilicon := family != "Intel"
	tePerCore := family != "M1" && family != "M2"
	singleKey := layoutOK && singleKeyLayout(layout)

	allKeys := make(map[string]struct{})

	for _, r := range results {
		for k := range r.deltas {
			allKeys[k] = struct{}{}
		}
	}

	coreKeys := make([]string, 0, len(allKeys))

	for k := range allKeys {
		if sk, _ := splitKey(k, appleSilicon); coreEligibleSeries(sk, appleSilicon, tePerCore) {
			coreKeys = append(coreKeys, k)
		}
	}

	seriesGroups := groupBySeries(coreKeys, appleSilicon)
	groups := make([][]string, 0, len(coreKeys))

	for _, sk := range sortedSeriesKeys(seriesGroups) {
		groups = append(groups, groupCores(seriesGroups[sk], appleSilicon, singleKey)...)
	}

	perPhase, clusterGroups := classifyCoreGroups(groups, results)

	var (
		moved              [][]string
		movedSrc, movedDst int
	)

	if layoutOK {
		moved, movedSrc, movedDst = rebalanceToLayout(perPhase, results, layout, groups)
	}

	clusterKeys := make([]string, 0, len(clusterGroups))

	for _, g := range clusterGroups {
		clusterKeys = append(clusterKeys, g...)
	}

	slices.Sort(clusterKeys)

	// Header.
	fmt.Println()
	fmt.Printf("// %s (%d logical CPUs) – guessed sensor mappings\n", family, numCPU)

	switch {
	case product.Name != "":
		fmt.Printf("// SKU: %s (%s)\n", product.CPU, product.Name)
	case product.CPU != "":
		fmt.Printf("// SKU: %s\n", product.CPU)
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

	if len(moved) > 0 {
		src := results[movedSrc]
		parts := make([]string, 0, len(moved))

		for _, g := range moved {
			parts = append(parts, fmt.Sprintf("%s +%.1f°C", g[0], groupMeanDelta(g, src.deltas)))
		}

		fmt.Printf("// NOTE: moved the %d weakest %s group(s) to %s to match the SKU layout "+
			"(their own heating was below the noise floor); verify those lines.\n"+
			"//       moved (mean %s-sweep rise per key): %s\n",
			len(moved), phaseMidWord(src.spec.label), phaseMidWord(results[movedDst].spec.label),
			phaseMidWord(src.spec.label), strings.Join(parts, ", "))
	}

	fmt.Printf("// %s\n", strings.Repeat("─", 72))

	// Per-phase sensor sections.
	for i, r := range results {
		printPhase(family, product.CPU, i, r, perPhase[i], layout, layoutOK, appleSilicon, tePerCore)
	}

	if layoutOK {
		warnAbsentTypes(layout, results, perPhase)
	}

	// Cluster / package sensors.
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
			if existing, ok := smc.LookupTempDesc(k, family, product.CPU); ok {
				fmt.Printf("%-50s ← src/temp.txt: %q\n", line, existing)
			} else {
				fmt.Println(line)
			}
		}
	}

	detected := allDetectedKeys(results, clusterKeys)
	printTempTxtCrosscheck(family, product.CPU, detected, visible)
	printReportsCrosscheck(family, product.CPU, detected)
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
// and the keys mapped in src/temp.txt for the family that this machine reads. The
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
func printTempTxtCrosscheck(family, sku string, detected map[string]struct{}, visible map[string]float32) {
	// Only keys this machine reads: the table also holds rows for other SKUs
	// and Intel-era All rows, which would otherwise swamp the silent list.
	mapped := smc.MappedTempKeys(family, sku)
	maps.DeleteFunc(mapped, func(k, _ string) bool {
		_, ok := visible[k]

		return !ok
	})

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

	slices.Sort(silent)

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
// observed in reports/ for the same SKU, matching the SKU-scoped temp.txt
// cross-check; it falls back to the whole family when no dump of the SKU exists. The reports represent ground-truth
// sensor presence captured from real machines, so missing detections often
// reflect runtime conditions (e.g. GPU/SSD sensors that don't heat from a
// CPU stress phase) rather than mapping bugs. Truly novel keys (detected this
// run but absent from every prior dump) are worth investigating — they may be
// new firmware additions or evidence the chip is a sub-variant we haven't
// captured in the reports/ set yet.
func printReportsCrosscheck(family, sku string, detected map[string]struct{}) {
	scope, observed := sku, reports.SKUKeys(sku)
	if len(observed) == 0 {
		scope, observed = family, reports.Keys(family)
	}

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

	slices.Sort(silent)

	novel := make([]string, 0)

	for k := range detected {
		if _, ok := observed[k]; !ok {
			novel = append(novel, k)
		}
	}

	slices.Sort(novel)

	fmt.Println()
	fmt.Printf("// %s\n", strings.Repeat("─", 72))
	fmt.Printf("// Cross-check vs reports/ observations for %q:\n", scope)
	fmt.Printf("//   detected: %d  |  observed: %d  |  matched: %d  |  novel: %d  |  silent: %d\n",
		len(detected), len(observed), matched, len(novel), len(silent))

	if len(novel) > 0 {
		const maxNovel = 20

		shown := min(len(novel), maxNovel)

		fmt.Printf("// Detected but never seen in any %s report (showing %d of %d):\n",
			scope, shown, len(novel))

		for _, k := range novel[:shown] {
			fmt.Printf("//   %s\n", k)
		}
	}
}
