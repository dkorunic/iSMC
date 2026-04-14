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
	"testing"

	"github.com/dkorunic/iSMC/platform"
	"github.com/dkorunic/iSMC/stress"
)

func TestSeriesKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		key  string
		want string
	}{
		{"TC0c", "TC*c"},
		{"TC3c", "TC*c"},
		{"TC9c", "TC*c"},
		{"Te0T", "Te*T"},
		{"Te1T", "Te*T"},
		{"Tp01", "Tp**"},
		{"Tp09", "Tp**"},
		{"Tf0c", "Tf*c"},
		{"TcXX", "TcXX"}, // no digits — returned unchanged
		{"Tp0A", "Tp**"}, // uppercase A is a hex digit → masked
		{"Tp0C", "Tp**"}, // uppercase C is a hex digit → masked
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			t.Parallel()

			got := seriesKey(tt.key)
			if got != tt.want {
				t.Errorf("seriesKey(%q) = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}

func TestNumericValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		key  string
		want int
	}{
		{"TC0c", 0},
		{"TC3c", 3},
		{"TC9c", 9},
		{"Tp01", 1},
		{"Tp09", 9},
		{"Te12", 12},
		{"TcXX", 0},  // no digits → 0
		{"Tp0A", 10}, // uppercase hex digit → 10
		{"Tp0C", 12}, // uppercase hex digit → 12
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			t.Parallel()

			got := numericValue(tt.key)
			if got != tt.want {
				t.Errorf("numericValue(%q) = %d, want %d", tt.key, got, tt.want)
			}
		})
	}
}

func TestGroupBySeries(t *testing.T) {
	t.Parallel()

	keys := []string{"TC2c", "TC0c", "Te1T", "TC1c", "Te0T"}
	got := groupBySeries(keys)

	if len(got) != 2 {
		t.Fatalf("groupBySeries: got %d series, want 2", len(got))
	}

	tcSeries, ok := got["TC*c"]
	if !ok {
		t.Fatal("groupBySeries: missing series TC*c")
	}

	wantTC := []string{"TC0c", "TC1c", "TC2c"}
	if len(tcSeries) != len(wantTC) {
		t.Fatalf("TC*c len: got %d, want %d; keys: %v", len(tcSeries), len(wantTC), tcSeries)
	}

	for i, k := range wantTC {
		if tcSeries[i] != k {
			t.Errorf("TC*c[%d] = %q, want %q", i, tcSeries[i], k)
		}
	}

	teSeries, ok := got["Te*T"]
	if !ok {
		t.Fatal("groupBySeries: missing series Te*T")
	}

	wantTe := []string{"Te0T", "Te1T"}
	if len(teSeries) != len(wantTe) {
		t.Fatalf("Te*T len: got %d, want %d; keys: %v", len(teSeries), len(wantTe), teSeries)
	}

	for i, k := range wantTe {
		if teSeries[i] != k {
			t.Errorf("Te*T[%d] = %q, want %q", i, teSeries[i], k)
		}
	}
}

func TestGroupByStrideWithinSeries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		sensors []string
		want    [][]string
	}{
		{
			name:    "single sensor",
			sensors: []string{"Tp00"},
			want:    [][]string{{"Tp00"}},
		},
		{
			name:    "two sensors uniform stride",
			sensors: []string{"Tp00", "Tp04"},
			want:    [][]string{{"Tp00"}, {"Tp04"}},
		},
		{
			name:    "M1 triplets stride-4 gap",
			sensors: []string{"Tp00", "Tp01", "Tp02", "Tp04", "Tp05", "Tp06", "Tp08", "Tp09", "Tp0A"},
			// numeric values: 0,1,2,4,5,6,8,9,10 → diffs: 1,1,2,1,1,2,1,1 — non-uniform, split at >1
			want: [][]string{
				{"Tp00", "Tp01", "Tp02"},
				{"Tp04", "Tp05", "Tp06"},
				{"Tp08", "Tp09", "Tp0A"},
			},
		},
		{
			name:    "M5 uniform stride-4",
			sensors: []string{"Tp00", "Tp04", "Tp08"},
			// numeric values: 0,4,8 → diffs: 4,4 — uniform → each sensor is its own group
			want: [][]string{{"Tp00"}, {"Tp04"}, {"Tp08"}},
		},
		{
			name:    "M4 irregular large gap",
			sensors: []string{"Tp00", "Tp01", "Tp02", "Tp04", "Tp05", "Tp06", "Tp08", "Tp09", "Tp21"},
			// numeric values: 0,1,2,4,5,6,8,9,21 → diffs: 1,1,2,1,1,2,1,12 — non-uniform, minDiff=1
			// splits at diffs > 1: after index 2 (diff=2), after index 5 (diff=2), after index 7 (diff=12)
			want: [][]string{
				{"Tp00", "Tp01", "Tp02"},
				{"Tp04", "Tp05", "Tp06"},
				{"Tp08", "Tp09"},
				{"Tp21"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := groupByStrideWithinSeries(tt.sensors)
			if len(got) != len(tt.want) {
				t.Fatalf("groupByStrideWithinSeries(%v): got %d groups, want %d; got=%v",
					tt.sensors, len(got), len(tt.want), got)
			}

			for i, wantGroup := range tt.want {
				if len(got[i]) != len(wantGroup) {
					t.Errorf("group[%d]: got %v (len %d), want %v (len %d)",
						i, got[i], len(got[i]), wantGroup, len(wantGroup))

					continue
				}

				for j, wantKey := range wantGroup {
					if got[i][j] != wantKey {
						t.Errorf("group[%d][%d] = %q, want %q", i, j, got[i][j], wantKey)
					}
				}
			}
		})
	}
}

func TestDeltaTemps(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		base map[string]float32
		hot  map[string]float32
		want map[string]float32
	}{
		{
			// delta=15.0 ≥ threshold=1.5 → included; TC1c delta=1.0 < threshold → excluded
			name: "one above one below threshold",
			base: map[string]float32{"TC0c": 30.0, "TC1c": 35.0},
			hot:  map[string]float32{"TC0c": 45.0, "TC1c": 36.0},
			want: map[string]float32{"TC0c": 15.0},
		},
		{
			// delta == threshold → included (>= comparison)
			name: "exactly at threshold",
			base: map[string]float32{"TC0c": 30.0},
			hot:  map[string]float32{"TC0c": 31.5},
			want: map[string]float32{"TC0c": 1.5},
		},
		{
			// key present only in base, not in hot → excluded
			name: "key missing from hot",
			base: map[string]float32{"TC0c": 30.0},
			hot:  map[string]float32{},
			want: map[string]float32{},
		},
		{
			// key present only in hot, not in base → excluded (range is over base)
			name: "key missing from base",
			base: map[string]float32{},
			hot:  map[string]float32{"TC0c": 45.0},
			want: map[string]float32{},
		},
		{
			name: "empty inputs",
			base: map[string]float32{},
			hot:  map[string]float32{},
			want: map[string]float32{},
		},
		{
			// Multiple sensors all above threshold
			name: "multiple sensors all above threshold",
			base: map[string]float32{"TC0c": 30.0, "TC1c": 32.0, "TC2c": 28.0},
			hot:  map[string]float32{"TC0c": 50.0, "TC1c": 45.0, "TC2c": 35.0},
			want: map[string]float32{"TC0c": 20.0, "TC1c": 13.0, "TC2c": 7.0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := deltaTemps(tt.base, tt.hot)

			if len(got) != len(tt.want) {
				t.Errorf("deltaTemps: got %d entries %v, want %d entries %v",
					len(got), got, len(tt.want), tt.want)

				return
			}

			for k, wantV := range tt.want {
				gotV, ok := got[k]
				if !ok {
					t.Errorf("deltaTemps: missing key %q in result", k)

					continue
				}

				if gotV != wantV {
					t.Errorf("deltaTemps[%q] = %g, want %g", k, gotV, wantV)
				}
			}
		})
	}
}

func TestPhaseMidWord(t *testing.T) {
	t.Parallel()

	tests := []struct {
		label string
		want  string
	}{
		// Standard three-word phase labels: return the middle word (parts[1])
		{"CPU Super Core", "Super"},
		{"CPU Performance Core", "Performance"},
		{"CPU Efficiency Core", "Efficiency"},
		// Single word: len(parts) < 2 → return label unchanged
		{"Single", "Single"},
		// Empty string: strings.Fields("") == [] → len < 2 → return ""
		{"", ""},
		// Two words: parts[1] is the second word
		{"Two Words", "Words"},
	}

	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			t.Parallel()

			got := phaseMidWord(tt.label)
			if got != tt.want {
				t.Errorf("phaseMidWord(%q) = %q, want %q", tt.label, got, tt.want)
			}
		})
	}
}

// TestBuildPhases verifies the phase specification construction for the three
// supported chip topologies:
//   - nil / empty perfLevels → 2 phases with zero core counts
//   - 2-level (M1–M4) → Performance + Efficiency with correct PhysicalCPU counts
//   - 3-level (M5+) → Super + Performance + Efficiency with correct PhysicalCPU counts
func TestBuildPhases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		levels     []platform.PerfLevel
		wantLabels []string
		wantCores  []int
		wantQoS    []int
	}{
		{
			name:       "nil levels → 2 phases with zero cores",
			levels:     nil,
			wantLabels: []string{"CPU Performance Core", "CPU Efficiency Core"},
			wantCores:  []int{0, 0},
			wantQoS:    []int{stress.QoSUserInitiated, stress.QoSBackground},
		},
		{
			name: "2-level chip (M1–M4)",
			levels: []platform.PerfLevel{
				{Name: "Performance", PhysicalCPU: 10, LogicalCPU: 10},
				{Name: "Efficiency", PhysicalCPU: 4, LogicalCPU: 4},
			},
			wantLabels: []string{"CPU Performance Core", "CPU Efficiency Core"},
			wantCores:  []int{10, 4},
			wantQoS:    []int{stress.QoSUserInitiated, stress.QoSBackground},
		},
		{
			name: "3-level chip (M5+)",
			levels: []platform.PerfLevel{
				{Name: "Super", PhysicalCPU: 4, LogicalCPU: 4},
				{Name: "Performance", PhysicalCPU: 8, LogicalCPU: 8},
				{Name: "Efficiency", PhysicalCPU: 4, LogicalCPU: 4},
			},
			wantLabels: []string{"CPU Super Core", "CPU Performance Core", "CPU Efficiency Core"},
			wantCores:  []int{4, 8, 4},
			wantQoS:    []int{stress.QoSUserInteractive, stress.QoSUserInitiated, stress.QoSBackground},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := buildPhases(tt.levels)

			if len(got) != len(tt.wantLabels) {
				t.Fatalf("buildPhases: got %d phases, want %d; phases=%v",
					len(got), len(tt.wantLabels), got)
			}

			for i, phase := range got {
				if phase.label != tt.wantLabels[i] {
					t.Errorf("buildPhases[%d].label = %q, want %q", i, phase.label, tt.wantLabels[i])
				}

				if phase.cores != tt.wantCores[i] {
					t.Errorf("buildPhases[%d].cores = %d, want %d", i, phase.cores, tt.wantCores[i])
				}

				if phase.qos != tt.wantQoS[i] {
					t.Errorf("buildPhases[%d].qos = %d, want %d", i, phase.qos, tt.wantQoS[i])
				}
			}
		})
	}
}

func TestSortedSeriesKeys(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		groups map[string][]string
		want   []string
	}{
		{
			name: "three series sorted lexicographically",
			groups: map[string][]string{
				"Tp**": {"Tp00", "Tp01"},
				"TC*c": {"TC0c", "TC1c"},
				"Te*T": {"Te0T"},
			},
			want: []string{"TC*c", "Te*T", "Tp**"},
		},
		{
			name:   "empty map",
			groups: map[string][]string{},
			want:   []string{},
		},
		{
			name: "single entry",
			groups: map[string][]string{
				"TC*c": {"TC0c"},
			},
			want: []string{"TC*c"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := sortedSeriesKeys(tt.groups)

			if len(got) != len(tt.want) {
				t.Fatalf("sortedSeriesKeys: got %v (len %d), want %v (len %d)",
					got, len(got), tt.want, len(tt.want))
			}

			for i, k := range tt.want {
				if got[i] != k {
					t.Errorf("sortedSeriesKeys[%d] = %q, want %q", i, got[i], k)
				}
			}
		})
	}
}
