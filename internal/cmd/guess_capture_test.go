// SPDX-FileCopyrightText: Copyright (C) 2026  Dinko Korunic
// SPDX-License-Identifier: GPL-3.0-only

//go:build darwin

package cmd

import (
	"errors"
	"math"
	"path/filepath"
	"reflect"
	"regexp"
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

// TestReplayM6Run replays a real `iSMC guess --gpu --save` run from a Mac mini
// M6 (v0.18.2). Its per-CPU load shows the 4-thread Performance phase also
// kept both Super cores busy, and its E-cores warmed by under 1 °C.
func TestReplayM6Run(t *testing.T) {
	c, err := loadCapture("testdata/m6-run.json")
	if err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() { analyzeCapture(c) })

	got := map[string][]string{}

	for _, m := range regexp.MustCompile(`(?m)^CPU (\w+) Core \d+:(\w+):`).FindAllStringSubmatch(out, -1) {
		got[m[1]] = append(got[m[1]], m[2])
	}

	want := map[string][]string{
		phaseMidWord(labelSuperCore):       {"Tp07", "Tp09", "Tp0g", "Tp0j"},
		phaseMidWord(labelPerformanceCore): {"Tp0E", "Tp0G", "Tp0I", "Tp0L"},
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("core lines = %v, want %v\n%s", got, want, out)
	}

	// The E phase's +0.6-0.9 °C rise passes the noise-scaled cutoff.
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
