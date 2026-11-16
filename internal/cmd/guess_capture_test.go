// SPDX-FileCopyrightText: Copyright (C) 2026  Dinko Korunic
// SPDX-License-Identifier: GPL-3.0-only

//go:build darwin

package cmd

import (
	"errors"
	"flag"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/dkorunic/iSMC/internal/platform"
)

// m6Capture is a cut-down M6 run: the Performance phase warms Tp0E, the
// Efficiency phase keeps CPUs 0-1 busy (shared with Super) without warming
// anything.
func m6Capture() *guessCapture {
	base := map[string]float32{"Tp07": 30, "Tp0E": 30, "TPD0": 31}
	layout, layoutOK := platform.LookupSKULayout("M6")
	phase := func(label string, hot map[string]float32, busy []float32) capturedPhase {
		return capturedPhase{
			Label: label, Threads: 2, Baseline: base, Hot: hot, CPUBusy: busy,
			Series: nil, QoS: 0, Cores: 0,
		}
	}

	return &guessCapture{
		Version: captureVersion, Tool: "test", Family: "M6", NumCPU: 4,
		Product: platform.Product{Name: "Mac mini (M6)", Family: "M6", CPU: "M6", Year: 2026},
		Layout:  layout, LayoutOK: layoutOK, PerfLevels: nil, GPU: nil,
		Phases: []capturedPhase{
			phase(labelSuperCore, map[string]float32{"Tp07": 38, "Tp0E": 33, "TPD0": 35},
				[]float32{0.99, 0.98, 0.05, 0.02}),
			phase(labelPerformanceCore, map[string]float32{"Tp07": 33, "Tp0E": 39, "TPD0": 35},
				[]float32{0.03, 0.04, 0.97, 0.3}),
			phase(labelEfficiencyCore, base, []float32{0.9, 0.95, 0, 0}),
		},
	}
}

func TestCaptureRoundTrip(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "run.json")

	err := saveCapture(path, m6Capture())
	if err != nil {
		t.Fatal(err)
	}

	c, err := loadCapture(path)
	if err != nil {
		t.Fatal(err)
	}

	if c.Family != "M6" || len(c.Phases) != 3 || c.Phases[1].Hot["Tp0E"] != 39 || c.Phases[2].CPUBusy[1] != 0.95 {
		t.Fatalf("round trip lost data: %+v", c)
	}

	c.Version++

	err = saveCapture(path, c)
	if err != nil {
		t.Fatal(err)
	}

	_, err = loadCapture(path)
	if !errors.Is(err, errCaptureVersion) {
		t.Errorf("loadCapture(other version) = %v, want errCaptureVersion", err)
	}
}

func TestPrintCPULoad(t *testing.T) {
	out := captureStdout(t, func() { printCPULoad(m6Capture()) })

	for _, want := range []string{
		"Phase 1 (Super, 2 thread(s)): CPUs 0 1\n",
		"Phase 2 (Performance, 2 thread(s)): CPUs 2 [3:30%]  ⚠ 1 busy CPU(s) for 2 thread(s)",
		"Phase 3 (Efficiency, 2 thread(s)): CPUs 0 1  ⚠ shares CPUs with phase 1:",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

var updateGolden = flag.Bool("update", false, "rewrite testdata/*.cores from the current classifier")

// TestReplayRuns replays real `iSMC guess --gpu --save` runs and compares the
// core lines with testdata/<run>.cores (refresh with -update after checking
// the diff). Each run catches a different failure:
//
//   - m6-run (Mac mini M6, v0.18.2): the 4-thread Performance phase also kept
//     both Super cores busy, Super cores have two lockstep probes each, and
//     the E-cores warm under 1 °C. Its four P "cores" are really two P-cores
//     with two probes each (only CPUs 8-9 were well loaded); src/temp.txt maps
//     all four P-cores from other data, and buildPhases now spins S+P threads.
//   - m1-ultra-run (Mac Studio M1 Ultra): 16 threads busied all 20 CPUs;
//     the E-cores are moved out of the Performance tier by rebalancing.
//   - m4-air-run (MacBook Air M4, Mac16,12, fanless): E load warms the whole
//     die 3-4 °C, so the end rise ties every P group between both phases; the
//     1 s onset tie-break (onsetPhase) puts them in the P tier. P-cores 1-4
//     match src/temp.txt; Tp3O/Tp3P and Tp3W/X are cluster maxima and drop
//     out. Known gap: the four P groups (Tp0U-Tp0f) rise exactly like the
//     P-cores and feed TCMz, so whole-tier loads cannot tell them apart.
//   - m4-pro-run (MacBook Pro 14 M4 Pro 10P+4E, Mac16,8, v0.18.4, iSMC#28):
//     the P-cores are reported through two key banks (Tp00..Tp0c + Tpx0-5,
//     Tp1i..Tp2E + Tpx8-D) that alternate every few seconds, the idle bank
//     parked at exactly 40.0 °C, which the sampler drops. Bank B lands in the
//     P tier; bank A has no rise because it was parked in the baseline. Its
//     triplets straddle index gaps (Tp1q/Tp1t/Tp1u) and are grouped by their
//     constant A/B offset (anchoredTriplets). Its E phase ran last and found
//     the die cooling, so no E key responded; assignTeByPrefix moves the Te*
//     groups to the E tier by key name (newer runs measure E first). Tex0-3
//     and TpxB-D copy keys of earlier groups (Tex1 = Te06, TpxB = Tp2R) and
//     Tpx9 is the maximum of the P-cores' third probes, so dropAliasGroups
//     lists them as cluster sensors. Known gap: the five fabric triplets
//     (Tp2I-Tp2X) rise only ~7% less than the cores and stay as extra P
//     groups.
//   - m3-air-run (MacBook Air 15 M3, Mac15,13, fanless, v0.18.4): sixteen P
//     triplets (Tp00-Tp10, B = A + 6.5) rise alike for 4 P-cores, in two banks
//     of eight whose maxima are Tp1F and Tp1L. The E phase warms the Te* keys
//     only ~2 °C, so rebalancing fills the E tier with the three Te* groups
//     plus Tp16. Tp1E/Tp1F drop out as a cluster sensor. Known gaps: all 16
//     triplets stay P groups; Tp1L (bank-2 max) and Tp1I (max of Tp14/Tp18)
//     match their members in only 83-90% of snapshots and stay P groups; the
//     Tp12/Tp16/Tp1H probes (+15.5 °C under P load, +21 °C under GPU load)
//     land as cores although they are neither tier.
//   - m1-ultra-ft-run (Mac Studio M1 Ultra, Mac13,2): the first run that also
//     reads the SMC ft index; its 20 ftP entries cover every core triplet, so
//     the core lines match m1-ultra-run's classification.
//   - m3-pro-run (MacBook Pro 14 M3 Pro 6P+6E, v0.18.4, saved without the ft
//     index): twelve P triplets rise +5.5-6.8 °C under P load, the probes
//     Tp0u-w/Tp0y-10 only ~3 °C (7.2-7.6 °C under GPU load). Known gaps
//     without the index: 12 P groups including the Tp3O/P and Tp3W/X bank
//     maxima, and rebalancing moves Tp0g, Tp0y and Tp1Q into the E tier.
//   - m5-air-run (MacBook Air 13 M5, Mac17,3, fanless, v0.18.4): eight
//     Super-level keys (Tp00-0G and Tp0O-0a, +7.9-8.7 °C, 1 s onset 0.7-0.9 °C)
//     for 4 Super cores; mergeLockstep pairs them, but 1 s snapshots make
//     every Super key correlate at r >= 0.99, so the pairing is not evidence.
//     Known gaps: Tp0p/Tp0y probes (+6 °C) stay as Super groups 5-6, and with
//     four Te groups for six E-cores rebalancing moves Tp0u/Tp1E into E.
//   - m3-max-run (MacBook Pro 16 M3 Max 12P+4E, Mac15,9, iSMC#34, dev build):
//     a second bank of 48 Tp keys (Tp1g-Tp3j), gated at 40.0 in every M3 Max
//     dump and in the baselines, goes live ~2 s into P load beside bank A;
//     ftPe-ftPr resolve to it, so most bank-A groups lose their index and drop
//     to cluster sensors (5 P groups). The E load warms nothing above noise.
//   - m4-max-run (Mac Studio M4 Max 12P+4E, Mac16,9, iSMC#34): both P banks
//     and ftP* read 40.0 (parked) at every baseline and wake ~1 s into P load,
//     so deltaTemps scores the whole Tp family against guessTempMin and
//     resolveFtIndex takes keys from every snapshot. Fabric 1-2 of each bank
//     read at core level (P-cores 11-12), Fabric 3-5 ~4.5 °C lower; the index
//     covers all 15 triplets per bank, so the run lists 30 P groups. The E
//     load warms nothing above noise.
//   - m5-max-run (MacBook Pro 16 M5 Max 6S+12P, Mac17,6, iSMC#34): every Tp*/
//     Tm* key idles below guessTempMin (~20 °C), so only the floor fallback
//     sees them. Super load heats all 18 Tp00-Tp0y keys (Super triplets) and
//     leaves Tm* idle; the P phase kept the Super CPUs busy (QoS did not
//     separate the tiers), and Tm* (P-core probes, +8 °C) are not core keys,
//     so SKU rebalancing moves 12 Super-triplet keys and the Tp1E-1g cluster
//     keys into the P tier.
//   - imac20-2-run (iMac 27 2020, Core i9-10910 10C/20T, iSMC#36, v0.18.5):
//     one perf level, but v0.18.5 still ran a Background phase on the same
//     20 CPUs; singlePerfLevel keeps the UserInitiated phase as plain cores.
//     TC2c read nothing at either baseline (reads fail intermittently, ~80 °C
//     under load), so it is the one missing core.
func TestReplayRuns(t *testing.T) {
	for _, name := range []string{"m6-run", "m1-ultra-run", "m4-air-run", "m4-pro-run", "m3-air-run", "m1-ultra-ft-run", "m3-pro-run", "m5-air-run", "m3-max-run", "m4-max-run", "m5-max-run", "imac20-2-run"} {
		t.Run(name, func(t *testing.T) {
			c, err := loadCapture("testdata/" + name + ".json")
			if err != nil {
				t.Fatal(err)
			}

			out := captureStdout(t, func() { analyzeCapture(c) })
			got := strings.Join(regexp.MustCompile(`(?m)^CPU (?:\w+ )?Core \d+:\w+:\w+`).FindAllString(out, -1), "\n") + "\n"
			golden := "testdata/" + name + ".cores"

			if *updateGolden {
				err = os.WriteFile(golden, []byte(got), 0o600)
				if err != nil {
					t.Fatal(err)
				}
			}

			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatal(err)
			}

			if got != string(want) {
				t.Errorf("core lines differ from %s:\ngot:\n%s\nwant:\n%s\nfull output:\n%s", golden, got, want, out)
			}
		})
	}
}

// TestReplayM6EPhase: the M6 E phase's +0.6-0.9 °C rise passes the
// noise-scaled cutoff (a fixed 1.0 °C cutoff dropped every E responder).
func TestReplayM6EPhase(t *testing.T) {
	c, err := loadCapture("testdata/m6-run.json")
	if err != nil {
		t.Fatal(err)
	}

	if e := deltaTemps(c.Phases[2].Baseline, c.Phases[2].Hot); e["Te08"] == 0 {
		t.Errorf("E phase lost Te08 (+0.86 °C); responders %v", e)
	}
}

func TestNoiseThreshold(t *testing.T) {
	t.Parallel()

	base := map[string]float32{"a": 30, "b": 30, "c": 30, "d": 30}

	for _, tc := range []struct {
		hot  map[string]float32
		want float32
	}{
		{map[string]float32{"a": 29.9, "b": 29.9, "c": 29.9, "d": 31}, guessOutputFloor}, // quiet keys
		{map[string]float32{"a": 29.8, "b": 29.8, "c": 29.8, "d": 31}, 0.6},              // 3 × 0.2
		{map[string]float32{"a": 29, "b": 29, "c": 29, "d": 31}, guessOutputThreshold},   // noisy: ceiling
		{map[string]float32{"a": 29.9, "b": 31, "c": 31, "d": 31}, guessOutputThreshold}, // no evidence
	} {
		if got := noiseThreshold(base, tc.hot); math.Abs(float64(got-tc.want)) > 1e-4 {
			t.Errorf("noiseThreshold(%v) = %v, want %v", tc.hot, got, tc.want)
		}
	}
}

// TestFindAggregates checks alias and maximum detection on real runs.
func TestFindAggregates(t *testing.T) {
	for _, tc := range []struct {
		run, key string
		members  []string
		maximum  bool
	}{
		// M6: one register under three keys.
		{"m6-run", "Te0f", []string{"Te0g", "Ts05"}, false},
		// M1 Ultra: the die-2 copies the allowlist could only suspect.
		{"m1-ultra-run", "TCMz", []string{"Tc02"}, false},
		// M6: TVNS is the hotter of TVN0/TVN1 on M5 too; here it equals TVN1.
		{"m6-run", "TVN1", []string{"TVNS"}, false},
		{"m1-ultra-run", "TVSx", []string{"TVS0", "TVS1"}, true},
	} {
		c, err := loadCapture("testdata/" + tc.run + ".json")
		if err != nil {
			t.Fatal(err)
		}

		aliases, maxima := findAggregates(captureSnapshots(c))

		rows := aliases
		if tc.maximum {
			rows = maxima
		}

		i := slices.IndexFunc(rows, func(a aggregate) bool { return a.Key == tc.key })
		if i < 0 || !slices.Equal(rows[i].Members, tc.members) {
			t.Errorf("%s: %s want %v, got aliases %v maxima %v", tc.run, tc.key, tc.members, aliases, maxima)
		}
	}
}
