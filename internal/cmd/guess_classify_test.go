// SPDX-FileCopyrightText: Copyright (C) 2026  Dinko Korunic
// SPDX-License-Identifier: GPL-3.0-only

//go:build darwin

package cmd

import (
	"slices"
	"strings"
	"testing"

	"github.com/dkorunic/iSMC/internal/platform"
)

// m1UltraDesktopRun reproduces the per-key deltas of a real Mac Studio (M1
// Ultra) guess run, cut down to two P-cores and one E-core. The E sweep warmed
// only the third key of each P-core by ~1.1 °C (noise), and the E-core never
// self-heated; it shows up only weakly in the P sweep.
func m1UltraDesktopRun() ([][]string, []phaseResult) {
	groups := [][]string{
		{"Tp00", "Tp01", "Tp02"}, // P-core 1
		{"Tp04", "Tp05", "Tp06"}, // P-core 2
		{"Tp08", "Tp09", "Tp0A"}, // E-core 1
	}
	results := []phaseResult{
		{
			spec: phaseSpec{label: labelPerformanceCore, qos: 0, cores: 2, threads: 2},
			deltas: map[string]float32{
				"Tp00": 7.6, "Tp01": 7.6, "Tp02": 6.1,
				"Tp04": 7.9, "Tp05": 7.9, "Tp06": 6.9,
				"Tp08": 3.7, "Tp09": 3.7, "Tp0A": 3.3,
			},
		},
		{
			spec:   phaseSpec{label: labelEfficiencyCore, qos: 0, cores: 1, threads: 1},
			deltas: map[string]float32{"Tp02": 1.1, "Tp06": 1.1},
		},
	}

	return groups, results
}

// TestClassifyCoreGroups_NoiseGuard: before the guard, the E sweep's 1.1 °C
// noise scored ~1.0 after median normalization, pulling P-core 1 into the E
// tier and P-core 2 into the cluster bucket.
func TestClassifyCoreGroups_NoiseGuard(t *testing.T) {
	t.Parallel()

	groups, results := m1UltraDesktopRun()

	perPhase, clusters := classifyCoreGroups(groups, results)

	if len(clusters) != 0 {
		t.Errorf("clusters = %v, want none", clusters)
	}

	if !slices.EqualFunc(perPhase[0], groups, slices.Equal) {
		t.Errorf("P phase = %v, want all three groups (E-core only responded there)", perPhase[0])
	}

	if len(perPhase[1]) != 0 {
		t.Errorf("E phase = %v, want empty", perPhase[1])
	}
}

// TestRebalanceToLayout: with the SKU saying 2P + 1E, the weakest P-sweep group
// (the E-core) moves to the E tier.
func TestRebalanceToLayout(t *testing.T) {
	t.Parallel()

	groups, results := m1UltraDesktopRun()
	perPhase, _ := classifyCoreGroups(groups, results)

	layout := platform.SKULayout{
		PairSignature: platform.PairSignaturePE,
		PCoresMin:     2, PCoresMax: 2,
		ECoresMin: 1, ECoresMax: 1,
		SCoresMin: 0, SCoresMax: 0,
		GPUCoresMin: 0, GPUCoresMax: 0,
		Dies: 1,
	}

	moved, src, dst := rebalanceToLayout(perPhase, results, layout, groups)

	if len(moved) != 1 || src != 0 || dst != 1 {
		t.Fatalf("rebalance moved %v from %d to %d, want the E-core from 0 to 1", moved, src, dst)
	}

	if !slices.EqualFunc(perPhase[0], groups[:2], slices.Equal) {
		t.Errorf("P phase = %v, want the two P-cores in order", perPhase[0])
	}

	if len(perPhase[1]) != 1 || !slices.Equal(perPhase[1][0], groups[2]) {
		t.Errorf("E phase = %v, want the E-core", perPhase[1])
	}

	// Counts already within the layout: nothing moves.
	if again, _, _ := rebalanceToLayout(perPhase, results, layout, groups); again != nil {
		t.Errorf("second rebalance moved %v, want nothing", again)
	}
}

// TestPrintTempTxtCrosscheck_VisibleOnly: rows for keys the machine does not
// read (Intel-era All rows, other SKUs) must not count as mapped or silent.
func TestPrintTempTxtCrosscheck_VisibleOnly(t *testing.T) {
	detected := map[string]struct{}{"Tp00": {}}
	visible := map[string]float32{"Tp00": 40, "Tp04": 40}

	out := captureStdout(t, func() { printTempTxtCrosscheck("M1", "M1 Pro", detected, visible) })

	if !strings.Contains(out, "mapped: 2  |  matched: 1  |  novel: 0  |  silent: 1") {
		t.Errorf("summary should count only visible keys:\n%s", out)
	}

	if strings.Contains(out, "TA0D") || !strings.Contains(out, "Tp04") {
		t.Errorf("silent list should be exactly Tp04:\n%s", out)
	}
}

// TestRebalanceToLayout_MeanNotSum: a P-core whose third key stayed under the
// output threshold (sum 9 over two keys) must not be moved ahead of a full
// E-core triplet (sum 10.7, but only ~3.6 per key).
func TestRebalanceToLayout_MeanNotSum(t *testing.T) {
	t.Parallel()

	groups, results := m1UltraDesktopRun()
	results[0].deltas["Tp04"], results[0].deltas["Tp05"] = 4.5, 4.5
	delete(results[0].deltas, "Tp06")

	perPhase := [][][]string{slices.Clone(groups), nil}
	layout := platform.SKULayout{
		PairSignature: platform.PairSignaturePE,
		PCoresMin:     2, PCoresMax: 2,
		ECoresMin: 1, ECoresMax: 1,
		SCoresMin: 0, SCoresMax: 0,
		GPUCoresMin: 0, GPUCoresMax: 0,
		Dies: 1,
	}

	moved, _, _ := rebalanceToLayout(perPhase, results, layout, groups)

	if len(moved) != 1 || moved[0][0] != "Tp08" {
		t.Errorf("moved %v, want the Tp08 E-core", moved)
	}
}

// TestRebalanceToLayout_SkipsEmptyPhase: a phase with no responders (M6 E phase)
// failed to load its tier, so rebalancing must not fill it with other cores.
func TestRebalanceToLayout_SkipsEmptyPhase(t *testing.T) {
	t.Parallel()

	groups, results := m1UltraDesktopRun()
	results[1].deltas = map[string]float32{}

	perPhase := [][][]string{slices.Clone(groups), nil}
	layout := platform.SKULayout{
		PairSignature: platform.PairSignaturePE,
		PCoresMin:     2, PCoresMax: 2,
		ECoresMin: 1, ECoresMax: 1,
		SCoresMin: 0, SCoresMax: 0,
		GPUCoresMin: 0, GPUCoresMax: 0,
		Dies: 1,
	}

	if moved, _, _ := rebalanceToLayout(perPhase, results, layout, groups); moved != nil {
		t.Fatalf("moved %v into a phase with no responders, want nothing", moved)
	}
}

// TestMeanDelta: per-key noise cancels in the signed mean, a cooling trend
// does not, so settledTemps waits out a chip that is still cooling.
func TestMeanDelta(t *testing.T) {
	t.Parallel()

	prev := map[string]float32{"a": 40, "b": 40, "c": 40, "d": 40}
	noise := map[string]float32{"a": 40.25, "b": 39.75, "c": 40.25, "d": 39.75}
	cooling := map[string]float32{"a": 39.8, "b": 39.8, "c": 39.8, "d": 39.8}

	if d := meanDelta(prev, noise); d != 0 {
		t.Errorf("noise: meanDelta = %v, want 0", d)
	}

	if d := meanDelta(prev, cooling); d > -guessSettleEpsilon {
		t.Errorf("cooling: meanDelta = %v, want below -%v (not settled)", d, guessSettleEpsilon)
	}
}
