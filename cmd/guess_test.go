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
