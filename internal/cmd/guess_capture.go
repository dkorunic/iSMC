// SPDX-FileCopyrightText: Copyright (C) 2026  Dinko Korunic
// SPDX-License-Identifier: GPL-3.0-only

//go:build darwin

package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/dkorunic/iSMC/internal/platform"
)

// captureVersion is bumped whenever guessCapture changes incompatibly.
const captureVersion = 1

var (
	errCaptureVersion = errors.New("unsupported guess run format version")
	errCaptureEmpty   = errors.New("guess run has no phases")
)

// CPU busy-share thresholds for the per-phase load report: a CPU at or above
// busyCPUShare carried a spinner; one at or above partialCPUShare is listed as
// spillover.
const (
	busyCPUShare    = float32(0.5)
	partialCPUShare = float32(0.1)
)

var (
	guessSave   string // --save: write the run to this JSON file.
	guessReplay string // --replay: analyse a saved run instead of measuring.
)

// guessCapture is everything a guess run measured, so a run captured on
// someone else's Mac can be re-analysed offline (--replay) and kept as a test
// fixture.
type guessCapture struct {
	GPU        *capturedPhase       `json:"gpu,omitempty"`
	Product    platform.Product     `json:"product"`
	Tool       string               `json:"tool"`
	Family     string               `json:"family"`
	PerfLevels []platform.PerfLevel `json:"perfLevels"`
	Phases     []capturedPhase      `json:"phases"`
	Layout     platform.SKULayout   `json:"layout"`
	Version    int                  `json:"version"`
	NumCPU     int                  `json:"numCpu"`
	LayoutOK   bool                 `json:"layoutOk"`
}

// capturedPhase is one stress phase: the settled baseline before it, the
// averaged peak sample, per-CPU busy share over the load window, and (with
// --save) the snapshots taken while the load ran.
type capturedPhase struct {
	Baseline map[string]float32 `json:"baseline"`
	Hot      map[string]float32 `json:"hot"`
	Label    string             `json:"label"`
	CPUBusy  []float32          `json:"cpuBusy,omitempty"`
	Series   []capturedSample   `json:"series,omitempty"`
	QoS      int                `json:"qos"`
	Cores    int                `json:"cores"`
	Threads  int                `json:"threads"`
}

// capturedSample is one snapshot taken T seconds into a phase's load.
type capturedSample struct {
	Temps map[string]float32 `json:"temps"`
	T     float32            `json:"t"`
}

// sampleDuring waits d; when record is set it snapshots every
// guessSeriesInterval meanwhile. Off by default: each snapshot is an SMC sweep
// on the sampling thread, which competes with the spinners for a core.
func (s *tempSampler) sampleDuring(d time.Duration, record bool) []capturedSample {
	if !record {
		time.Sleep(d)

		return nil
	}

	start := time.Now()

	var series []capturedSample

	//nolint:wsl_v5 // loop header reads t; nothing to separate.
	for t := time.Since(start); t < d; t = time.Since(start) {
		series = append(series, capturedSample{T: float32(t.Seconds()), Temps: s.rawTemps()})
		time.Sleep(min(guessSeriesInterval, d-time.Since(start)))
	}

	return series
}

func saveCapture(path string, c *guessCapture) error {
	b, err := json.MarshalIndent(c, "", " ")
	if err != nil {
		return fmt.Errorf("encode guess run: %w", err)
	}

	err = os.WriteFile(path, b, 0o600)
	if err != nil {
		return fmt.Errorf("save guess run: %w", err)
	}

	return nil
}

func loadCapture(path string) (*guessCapture, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read guess run: %w", err)
	}

	var c guessCapture

	err = json.Unmarshal(b, &c)
	if err != nil {
		return nil, fmt.Errorf("decode guess run %s: %w", path, err)
	}

	if c.Version != captureVersion {
		return nil, fmt.Errorf("%s: %w %d (want %d)", path, errCaptureVersion, c.Version, captureVersion)
	}

	if len(c.Phases) == 0 {
		return nil, fmt.Errorf("%s: %w", path, errCaptureEmpty)
	}

	return &c, nil
}

// printGuessHeader prints the platform summary shown before a live run and at
// the top of a replay.
func printGuessHeader(c *guessCapture, phases int) {
	fmt.Printf("Platform : %s\n", c.Family)

	switch {
	case c.Product.Name != "":
		fmt.Printf("Model    : %s (%s)\n", c.Product.Name, c.Product.CPU)
	case c.Product.CPU != "":
		fmt.Printf("Model    : %s\n", c.Product.CPU)
	}

	if c.LayoutOK {
		fmt.Printf("SKU      : %s, %d die(s)\n", c.Layout.PairSignature, c.Layout.Dies)
		fmt.Printf("Expected : %s\n", layoutSummary(c.Layout))
	}

	fmt.Printf("CPUs     : %d logical\n", c.NumCPU)
	fmt.Printf("Per-phase: %v stress  (×%d phases, one spinner per target-tier CPU)\n\n",
		guessStressDuration, phases)

	if len(c.PerfLevels) > 0 {
		fmt.Printf("Topology : %d perf level(s)\n", len(c.PerfLevels))

		for i, pl := range c.PerfLevels {
			fmt.Printf("           perflevel%d: %d phys-CPU %q\n", i, pl.PhysicalCPU, pl.Name)
		}

		fmt.Println()
	}
}

// analyzeCapture classifies a live or replayed run and prints the mapping.
func analyzeCapture(c *guessCapture) {
	results := make([]phaseResult, 0, len(c.Phases))

	for _, p := range c.Phases {
		results = append(results, phaseResult{
			spec:   phaseSpec{label: p.Label, qos: p.QoS, cores: p.Cores, threads: p.Threads},
			deltas: deltaTemps(p.Baseline, p.Hot),
			series: p.Series,
		})
	}

	for _, note := range subtractNestedPhases(c, results) {
		fmt.Println(note)
	}

	// The last settled baseline stands for "what this machine reads".
	visible := c.Phases[len(c.Phases)-1].Baseline
	if c.GPU != nil {
		visible = c.GPU.Baseline
	}

	printMapping(c.Family, c.NumCPU, c.PerfLevels, c.Layout, c.LayoutOK, c.Product, results, visible)
	printCPULoad(c)

	if c.GPU != nil {
		printGPUPhase(c.Family, c.Product.CPU, deltaTemps(c.GPU.Baseline, c.GPU.Hot), visible, results)
	}
}

// subtractNestedPhases makes the per-CPU load count in classification. When a
// phase kept busy every CPU another phase did, plus more, its rises include
// that other phase's load (M6: the 4-thread Performance phase also kept both
// Super cores at 100%), so the larger phase is scored on its rise minus the
// smaller phase's. With several nested phases the largest one is subtracted.
// It returns a NOTE line per adjusted phase.
func subtractNestedPhases(c *guessCapture, results []phaseResult) []string {
	busy := make([][]int, len(c.Phases))
	for i, p := range c.Phases {
		busy[i] = busyCPUs(p.CPUBusy, busyCPUShare)
	}

	var notes []string

	for j, pj := range c.Phases {
		sub := -1

		for i := range c.Phases {
			if i != j && len(busy[i]) > 0 && len(busy[i]) < len(busy[j]) &&
				isSubset(busy[i], busy[j]) && (sub < 0 || len(busy[i]) > len(busy[sub])) {
				sub = i
			}
		}

		if sub < 0 {
			continue
		}

		pi := c.Phases[sub]
		thr := noiseThreshold(pj.Baseline, pj.Hot)
		d := make(map[string]float32)

		for k, b := range pj.Baseline {
			h, ok := pj.Hot[k]
			if !ok {
				continue
			}

			if v := (h - b) - (pi.Hot[k] - pi.Baseline[k]); v >= thr {
				d[k] = v
			}
		}

		results[j].deltas = d
		notes = append(notes, fmt.Sprintf("// NOTE: phase %d (%s) also kept phase %d's CPUs busy (%s); "+
			"it is scored on its rise minus phase %d's.", j+1, phaseMidWord(pj.Label), sub+1,
			strings.Trim(fmt.Sprint(busy[sub]), "[]"), sub+1))
	}

	return notes
}

// isSubset reports whether every element of a is in b.
func isSubset(a, b []int) bool {
	for _, x := range a {
		if !slices.Contains(b, x) {
			return false
		}
	}

	return true
}

// printCPULoad reports which logical CPUs each phase actually kept busy. QoS is
// only a hint, so this is the check that a phase loaded the tier it is named
// after: a phase sharing busy CPUs with another did not separate the tiers, and
// one with busy CPUs but no responders ran its tier without warming it.
func printCPULoad(c *guessCapture) {
	if !slices.ContainsFunc(c.Phases, func(p capturedPhase) bool { return len(p.CPUBusy) > 0 }) {
		return
	}

	fmt.Println()
	fmt.Printf("// %s\n", strings.Repeat("─", 72))
	fmt.Printf("// CPU load per phase (busy ≥ %.0f%%; spillover ≥ %.0f%% in brackets):\n",
		busyCPUShare*100, partialCPUShare*100)

	owner := map[int]int{} // CPU → first phase that kept it busy.

	for i, p := range c.Phases {
		busy := busyCPUs(p.CPUBusy, busyCPUShare)

		var spill []string

		for cpu, share := range p.CPUBusy {
			if share >= partialCPUShare && share < busyCPUShare {
				spill = append(spill, fmt.Sprintf("%d:%.0f%%", cpu, share*100))
			}
		}

		line := fmt.Sprintf("//   Phase %d (%s, %d thread(s)): CPUs %s", i+1, phaseMidWord(p.Label), p.Threads,
			strings.Trim(fmt.Sprint(busy), "[]"))
		if len(busy) == 0 {
			line += "none"
		}

		if len(spill) > 0 {
			line += " [" + strings.Join(spill, " ") + "]"
		}

		var shared []int

		for _, cpu := range busy {
			if j, ok := owner[cpu]; ok {
				shared = append(shared, j+1)
			} else {
				owner[cpu] = i
			}
		}

		switch {
		case len(shared) > 0:
			line += "  ⚠ shares CPUs with phase " +
				strings.Trim(fmt.Sprint(slices.Compact(slices.Sorted(slices.Values(shared)))), "[]") +
				": QoS did not separate the tiers"
		case len(busy) != p.Threads:
			line += fmt.Sprintf("  ⚠ %d busy CPU(s) for %d thread(s): threads migrated or were throttled",
				len(busy), p.Threads)
		}

		fmt.Println(line)
	}
}

// busyCPUs returns the CPU numbers whose busy share is at least minShare.
func busyCPUs(shares []float32, minShare float32) []int {
	var out []int

	for cpu, share := range shares {
		if share >= minShare {
			out = append(out, cpu)
		}
	}

	return out
}
