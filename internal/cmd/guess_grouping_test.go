// SPDX-FileCopyrightText: Copyright (C) 2026  Dinko Korunic
// SPDX-License-Identifier: GPL-3.0-only

//go:build darwin

package cmd

import (
	"slices"
	"testing"
)

func TestCharValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		c    byte
		want int
	}{
		{'0', 0},
		{'9', 9},
		{'A', 10},
		{'F', 15},
		{'Z', 35},
		{'a', 36},
		{'y', 60},
		{'z', 61},
		{'%', -1},
		{'*', -1},
	}

	for _, tt := range tests {
		t.Run(string(tt.c), func(t *testing.T) {
			t.Parallel()

			if got := charValue(tt.c); got != tt.want {
				t.Errorf("charValue(%q) = %d, want %d", tt.c, got, tt.want)
			}
		})
	}
}

func TestSplitKeyAppleSilicon(t *testing.T) {
	t.Parallel()

	tests := []struct {
		key        string
		wantSeries string
		wantIndex  int
	}{
		// Lowercase-second-char families use a 2-char positional base-62 index.
		{"Tp00", "Tp**", 0},
		{"Tp09", "Tp**", 9},
		{"Tp0A", "Tp**", 10}, // 'A' follows '9' positionally, no hex jump
		{"Tp0C", "Tp**", 12},
		{"Tp0y", "Tp**", 60},
		{"Tp0z", "Tp**", 61},
		{"Tp10", "Tp**", 62}, // consecutive after Tp0z (M3 P9 triplet 0y/0z/10)
		{"Tp29", "Tp**", 133},
		{"Tp2A", "Tp**", 134}, // adjacent to Tp29 — regression for the hex-radix bug
		{"Te0T", "Te**", 29},
		{"Th2G", "Th**", 140},
		{"Ts0P", "Ts**", 25},
		// Uppercase-second-char families keep the classic digits-only index.
		{"TC10", "TC**", 10},
		{"TC63", "TC**", 63},
		{"TS0P", "TS*P", 0},
		{"TVD0", "TVD*", 0},
		{"TCDX", "TCDX", 0},
		{"TCMz", "TCMz", 0},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			t.Parallel()

			series, index := splitKey(tt.key, true)
			if series != tt.wantSeries || index != tt.wantIndex {
				t.Errorf("splitKey(%q, apple) = (%q, %d), want (%q, %d)",
					tt.key, series, index, tt.wantSeries, tt.wantIndex)
			}
		})
	}
}

func TestSplitKeyIntel(t *testing.T) {
	t.Parallel()

	tests := []struct {
		key        string
		wantSeries string
		wantIndex  int
	}{
		// Intel/T2 keys are classic-shaped even with lowercase family letters:
		// digit index at position 2, probe letter at position 3.
		{"Tc0a", "Tc*a", 0},
		{"Tc7a", "Tc*a", 7},
		{"Tp2a", "Tp*a", 2},
		{"Tp9z", "Tp*z", 9},
		{"Th1b", "Th*b", 1},
		{"TC0P", "TC*P", 0},
		{"TB0T", "TB*T", 0},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			t.Parallel()

			series, index := splitKey(tt.key, false)
			if series != tt.wantSeries || index != tt.wantIndex {
				t.Errorf("splitKey(%q, intel) = (%q, %d), want (%q, %d)",
					tt.key, series, index, tt.wantSeries, tt.wantIndex)
			}
		})
	}
}

// groupsOf converts a [][]string result to key counts for compact assertions.
func groupsOf(groups [][]string) []int {
	sizes := make([]int, 0, len(groups))
	for _, g := range groups {
		sizes = append(sizes, len(g))
	}

	return sizes
}

func TestGroupCoresM1UltraTriplets(t *testing.T) {
	t.Parallel()

	// Full M1 Ultra per-core Tp set from src/temp.txt: 16 P + 4 E cores,
	// one triplet each, across two dies.
	suffixes := []string{
		"00", "01", "02", "04", "05", "06", "08", "09", "0A", "0C", "0D", "0E",
		"0G", "0H", "0I", "0K", "0L", "0M", "0O", "0P", "0Q", "0S", "0T", "0U",
		"0W", "0X", "0Y", "0a", "0b", "0c",
		"20", "21", "22", "24", "25", "26", "28", "29", "2A", "2C", "2D", "2E",
		"2G", "2H", "2I", "2K", "2L", "2M", "2O", "2P", "2Q", "2S", "2T", "2U",
		"2W", "2X", "2Y", "2a", "2b", "2c",
	}

	keys := make([]string, 0, len(suffixes))
	for _, s := range suffixes {
		keys = append(keys, "Tp"+s)
	}

	groups := groupCores(keys, true, false)

	if len(groups) != 20 {
		t.Fatalf("M1 Ultra: got %d core groups, want 20; sizes=%v", len(groups), groupsOf(groups))
	}

	for i, g := range groups {
		if len(g) != 3 {
			t.Errorf("group %d: got %v, want a triplet", i, g)
		}
	}

	// Regression: the die-2 E-core 3 triplet must stay together.
	want := []string{"Tp28", "Tp29", "Tp2A"}
	found := slices.ContainsFunc(groups, func(g []string) bool {
		return slices.Equal(g, want)
	})

	if !found {
		t.Errorf("triplet %v not found intact; groups=%v", want, groups)
	}
}

func TestGroupCoresM4BaseContiguousRun(t *testing.T) {
	t.Parallel()

	// M4 base populates a contiguous 12-key run Tp0U..Tp0f (no gaps) holding
	// four triplets; triplet-convention chunking must recover the alignment.
	keys := []string{
		"Tp00", "Tp01", "Tp02", "Tp04", "Tp05", "Tp06",
		"Tp08", "Tp09", "Tp0A", "Tp0C", "Tp0D", "Tp0E",
		"Tp0U", "Tp0V", "Tp0W", "Tp0X", "Tp0Y", "Tp0Z",
		"Tp0a", "Tp0b", "Tp0c", "Tp0d", "Tp0e", "Tp0f",
	}

	groups := groupCores(keys, true, false)

	if len(groups) != 8 {
		t.Fatalf("M4 base: got %d core groups, want 8; sizes=%v", len(groups), groupsOf(groups))
	}

	wantAligned := [][]string{
		{"Tp0U", "Tp0V", "Tp0W"},
		{"Tp0X", "Tp0Y", "Tp0Z"},
		{"Tp0a", "Tp0b", "Tp0c"},
		{"Tp0d", "Tp0e", "Tp0f"},
	}
	for _, want := range wantAligned {
		found := slices.ContainsFunc(groups, func(g []string) bool {
			return slices.Equal(g, want)
		})
		if !found {
			t.Errorf("aligned triplet %v not found; groups=%v", want, groups)
		}
	}
}

func TestGroupCoresM5SingleKey(t *testing.T) {
	t.Parallel()

	// M5 Pro/Max: 6 S-cores at stride 4, 12 P-cores at stride 3-5; every key
	// is one core (single-key convention) — no triplet chunking.
	keys := []string{
		"Tp00", "Tp04", "Tp08", "Tp0C", "Tp0G", "Tp0K",
		"Tp0O", "Tp0R", "Tp0U", "Tp0X", "Tp0a", "Tp0d",
		"Tp0g", "Tp0j", "Tp0m", "Tp0p", "Tp0u", "Tp0y",
	}

	groups := groupCores(keys, true, true)

	if len(groups) != 18 {
		t.Fatalf("M5: got %d core groups, want 18; sizes=%v", len(groups), groupsOf(groups))
	}

	for i, g := range groups {
		if len(g) != 1 {
			t.Errorf("group %d: got %v, want a single key", i, g)
		}
	}
}

func TestCoreEligibleSeries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		series    string
		apple     bool
		tePerCore bool
		want      bool
	}{
		{"Tp**", true, true, true},
		{"Te**", true, true, true},   // M3/M4/A18: Te* are per-core E sensors
		{"Te**", true, false, false}, // M1/M2: Te* are die/cluster aggregates
		{"Tp**", true, false, true},
		{"Ts**", true, true, false},  // SSD
		{"Th**", true, true, false},  // heatsink
		{"Tg**", true, true, false},  // GPU
		{"TC**", true, true, false},  // cluster diode grid
		{"TS*P", true, true, false},  // SSD proximity
		{"TCDX", true, true, false},  // die aggregate
		{"TCMz", true, true, false},  // die max
		{"TVD*", true, true, false},  // virtual
		{"Tc*a", false, true, true},  // Intel/T2 per-core quad
		{"Tc*x", false, true, true},  // Intel/T2 per-core quad
		{"TC*C", false, true, true},  // Intel classic per-core
		{"TC*c", false, true, true},  // Intel classic per-core
		{"Tp*a", false, true, false}, // Intel powerboard, not a core
		{"Th*b", false, true, false}, // Intel heatpipe
	}

	for _, tt := range tests {
		t.Run(tt.series, func(t *testing.T) {
			t.Parallel()

			got := coreEligibleSeries(tt.series, tt.apple, tt.tePerCore)
			if got != tt.want {
				t.Errorf("coreEligibleSeries(%q, apple=%v, tePerCore=%v) = %v, want %v",
					tt.series, tt.apple, tt.tePerCore, got, tt.want)
			}
		})
	}
}

func TestClassifyCoreGroups(t *testing.T) {
	t.Parallel()

	// Thermal-coupling scenario from a real M1 Ultra run: during the P-phase
	// the whole die heats, so E-core sensors show sizeable absolute deltas
	// there — larger than their own self-heating delta in the E-phase.
	// Median-normalized scoring must still assign them to the E-phase.
	pPhase := phaseResult{
		spec: phaseSpec{label: labelPerformanceCore},
		deltas: map[string]float32{
			"Tp00": 3, "Tp01": 3, "Tp02": 2, // P core 1: sum 8
			"Tp04": 3, "Tp05": 3, "Tp06": 2, // P core 2: sum 8
			"Tp08": 1.5, "Tp09": 1.5, "Tp0A": 1, // E core 1: die heating, sum 4
			"TCDX": 6, // package aggregate
		},
	}
	ePhase := phaseResult{
		spec: phaseSpec{label: labelEfficiencyCore},
		deltas: map[string]float32{
			"Tp08": 1, "Tp09": 1, "Tp0A": 1, // E core 1: self-heating, sum 3
			"TCDX": 2.4, // package aggregate follows both phases
		},
	}

	groups := [][]string{
		{"Tp00", "Tp01", "Tp02"},
		{"Tp04", "Tp05", "Tp06"},
		{"Tp08", "Tp09", "Tp0A"},
		{"TCDX"},
	}

	perPhase, clusters := classifyCoreGroups(groups, []phaseResult{pPhase, ePhase})

	if len(perPhase) != 2 {
		t.Fatalf("perPhase: got %d phases, want 2", len(perPhase))
	}

	if len(perPhase[0]) != 2 ||
		!slices.Equal(perPhase[0][0], groups[0]) ||
		!slices.Equal(perPhase[0][1], groups[1]) {
		t.Errorf("phase 0: got %v, want the two P-core groups", perPhase[0])
	}

	if len(perPhase[1]) != 1 || !slices.Equal(perPhase[1][0], groups[2]) {
		t.Errorf("phase 1: got %v, want the E-core group", perPhase[1])
	}

	if len(clusters) != 1 || !slices.Equal(clusters[0], groups[3]) {
		t.Errorf("clusters: got %v, want the TCDX group", clusters)
	}
}
