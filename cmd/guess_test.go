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
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			got := seriesKey(tt.key)
			if got != tt.want {
				t.Errorf("seriesKey(%q) = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}

func TestNumericValue(t *testing.T) {
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
		{"TcXX", 0}, // no digits → 0
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			got := numericValue(tt.key)
			if got != tt.want {
				t.Errorf("numericValue(%q) = %d, want %d", tt.key, got, tt.want)
			}
		})
	}
}

func TestGroupBySeries(t *testing.T) {
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
