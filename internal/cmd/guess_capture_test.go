// SPDX-FileCopyrightText: Copyright (C) 2026  Dinko Korunic
// SPDX-License-Identifier: GPL-3.0-only

//go:build darwin

package cmd

import (
	"errors"
	"path/filepath"
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
