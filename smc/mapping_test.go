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

package smc

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// m4Pro14CoreReport is a representative key→value snapshot captured from a real
// M4 Pro 14-core (10 Performance + 4 Efficiency cores) SMC report. Sentinel
// values (−4, 0, 2.2, 3.4, 5.2 °C) observed in Tp0* groups 0-7 are included
// to verify they are rejected by isValidReading.
var m4Pro14CoreReport = map[string]float32{
	// ── Efficiency Core 1 (Te04/05/06 triplet) ─────────────────────────────
	"Te04": 35.33, "Te05": 41.13, "Te06": 44.81,
	// ── Efficiency Core 2 (Te0R/S/T triplet) ──────────────────────────────
	"Te0R": 34.92, "Te0S": 40.52, "Te0T": 43.50,
	// ── Efficiency Cores 3-4 (Tpx8-D; Te08-Te0I absent on M4 Pro 14-core) ─
	"Tpx8": 39.13, "Tpx9": 48.25, "TpxA": 38.88, // E-core 3
	"TpxB": 46.05, "TpxC": 39.04, "TpxD": 51.16, // E-core 4
	// ── Efficiency cluster die aggregates ──────────────────────────────────
	"Tex0": 39.33, "Tex1": 44.81, // Die 1
	"Tex2": 38.92, "Tex3": 43.50, // Die 2
	// ── P-core Tp1*/Tp2* scheme: exactly 10 P-core triplets ────────────────
	"Tp1i": 34.81, "Tp1j": 42.21, "Tp1k": 45.20, // Core 1
	"Tp1m": 34.98, "Tp1n": 41.18, "Tp1o": 48.25, // Core 2
	"Tp1q": 34.46, "Tp1t": 41.86, "Tp1u": 43.36, // Core 3
	"Tp1v": 34.52, "Tp1w": 40.72, "Tp1x": 45.23, // Core 4
	"Tp1y": 34.75, "Tp1z": 42.15, "Tp20": 46.38, // Core 5
	"Tp21": 34.66, "Tp22": 40.86, "Tp23": 49.42, // Core 6
	"Tp24": 34.60, "Tp25": 42.00, "Tp26": 47.58, // Core 7
	"Tp27": 35.04, "Tp28": 41.24, "Tp29": 51.16, // Core 8
	"Tp2A": 35.70, "Tp2B": 43.10, "Tp2C": 44.58, // Core 9
	"Tp2D": 35.91, "Tp2E": 42.11, "Tp2G": 46.72, // Core 10
	// ── Tp0* Core 9 override (Tp0W/X/Y, Tp0Z absent) ──────────────────────
	"Tp0W": 34.19, "Tp0X": 41.59, "Tp0Y": 42.20,
	// ── Tp0* Core 10 override (Tp0a sentinel, Tp0b below minTempCelsius) ───
	"Tp0a": -4.0, "Tp0b": 2.2, "Tp0c": 41.94,
	// ── Tp0* groups 0-7: sentinel values (must be rejected, keys removed from
	//    M4 sensor table but kept here to catch re-addition regressions) ────
	"Tp00": -4.0, "Tp01": 3.4, "Tp02": 0.0,
	"Tp04": -4.0, "Tp05": 2.2, "Tp06": 0.0,
	"Tp08": -4.0, "Tp09": 3.4, "Tp0A": 0.0,
	"Tp0C": -4.0, "Tp0D": 2.2, "Tp0E": 0.0,
	"Tp0G": -4.0, "Tp0H": 3.4, "Tp0I": 0.0,
	"Tp0K": -4.0, "Tp0L": 2.2, "Tp0M": 0.0,
	"Tp0O": -4.0, "Tp0P": 3.4, "Tp0Q": 0.0,
	"Tp0S": -4.0, "Tp0T": 2.2, "Tp0U": 0.0,
	// ── Phantom P-cores 11-15 (removed from M4 table; present here to catch
	//    re-addition regressions; values are real-looking but not physical P-cores) ─
	"Tp2I": 34.89, "Tp2J": 42.29, "Tp2K": 42.61, // was phantom Core 11
	"Tp2L": 34.94, "Tp2M": 41.14, "Tp2N": 42.30, // was phantom Core 12
	"Tp2O": 35.72, "Tp2Q": 44.92, "Tp2R": 46.75, // was phantom Core 13
	"Tp2S": 35.60, "Tp2T": 44.80, "Tp2U": 46.70, // was phantom Core 14
	"Tp2V": 35.40, "Tp2W": 44.60, "Tp2X": 46.44, // was phantom Core 15
	// ── Phantom Core 11 from Tp0d/e/f (removed from M4 table) ─────────────
	"Tp0d": 34.70, "Tp0e": 42.10, "Tp0f": 42.45,
}

// resolveM4Sensors simulates the addGeneric pipeline for a given set of sensors
// and a key→value snapshot, without requiring a live SMC connection.
// It applies the same isValidReading filter and last-write-wins semantics as the
// production getGeneric+addGeneric path.
func resolveM4Sensors(sensors []SensorStat, snapshot map[string]float32) map[string]float32 {
	out := make(map[string]float32)

	for _, s := range sensors {
		val, present := snapshot[s.Key]
		if !present {
			continue
		}

		if isValidReading(val, TempUnit) {
			out[s.Desc] = val
		}
	}

	return out
}

// Test_M4Pro14CoreMapping verifies that the M4 temperature sensor definitions in
// AppleTemp, combined with the isValidReading filter, resolve the M4 Pro 14-core
// SMC report snapshot to exactly 10 Performance Cores and 4 Efficiency Cores.
//
// It also asserts:
//   - every resolved core temperature is within a plausible idle/load range
//   - no phantom P-cores (11-15, from Tp2I-Tp2X) appear, even though those keys
//     return real-looking values in the snapshot
//   - Tp0* sentinel values (−4, 0, 2.2, 3.4 °C) do not overwrite valid readings
func Test_M4Pro14CoreMapping(t *testing.T) {
	// Extract only M4-tagged sensors from the generated AppleTemp table.
	m4Sensors := make([]SensorStat, 0, 64)

	for _, s := range AppleTemp {
		if s.Platform == "M4" {
			m4Sensors = append(m4Sensors, s)
		}
	}

	resolved := resolveM4Sensors(m4Sensors, m4Pro14CoreReport)

	// Count distinct P-core and E-core descriptions.
	var (
		pCores     int
		eCores     int
		pCoreNames []string
		eCoreNames []string
	)

	for desc := range resolved {
		switch {
		case strings.HasPrefix(desc, "CPU Performance Core "):
			pCores++
			pCoreNames = append(pCoreNames, desc)
		case strings.HasPrefix(desc, "CPU Efficiency Core "):
			eCores++
			eCoreNames = append(eCoreNames, desc)
		}
	}

	assert.Equal(t, 10, pCores,
		"M4 Pro 14-core must resolve to exactly 10 Performance Cores; got %v", pCoreNames)
	assert.Equal(t, 4, eCores,
		"M4 Pro 14-core must resolve to exactly 4 Efficiency Cores; got %v", eCoreNames)

	// Verify each P-core has a plausible temperature (not a sentinel, not implausibly high).
	for _, name := range pCoreNames {
		temp := resolved[name]
		assert.GreaterOrEqual(t, temp, float32(minTempCelsius),
			"%s temperature %g °C is below minTempCelsius — sentinel not rejected", name, temp)
		assert.Less(t, temp, float32(110.0),
			"%s temperature %g °C is implausibly high", name, temp)
	}

	// Verify each E-core has a plausible temperature.
	for _, name := range eCoreNames {
		temp := resolved[name]
		assert.GreaterOrEqual(t, temp, float32(minTempCelsius),
			"%s temperature %g °C is below minTempCelsius", name, temp)
		assert.Less(t, temp, float32(110.0),
			"%s temperature %g °C is implausibly high", name, temp)
	}

	// Explicitly assert that no phantom P-cores 11-15 appeared.
	for i := 11; i <= 15; i++ {
		phantom := fmt.Sprintf("CPU Performance Core %d", i)
		_, found := resolved[phantom]
		assert.False(t, found, "phantom %q must not appear in resolved sensors", phantom)
	}
}

// a18ProReport is a representative key→value snapshot captured from report-a18.txt
// (MacBook Neo, A18 Pro, 2 Performance + 4 Efficiency cores). It exercises the
// hybrid sensor scheme: M1-style Tp0x triplets for P-cores, M3/M4-style Te triplets
// plus lowercase/non-contiguous Tp0* triplets for E-cores, and cluster-level
// aggregates at Tp08-0E, Tpx0-3, Tex0-3.
var a18ProReport = map[string]float32{
	// ── Performance Core 1 & 2 (M1-style Tp0x triplets) ───────────────────
	"Tp00": 85.63, "Tp01": 92.76, "Tp02": 106.30,
	"Tp04": 87.36, "Tp05": 94.28, "Tp06": 110.56,
	// ── Performance Cluster aggregates (must not be counted as cores) ─────
	"Tp08": 86.01, "Tp09": 91.43, "Tp0A": 106.23,
	"Tp0C": 88.18, "Tp0D": 96.10, "Tp0E": 111.91,
	// ── Efficiency Core 1 (Te04/05/06 triplet) ────────────────────────────
	"Te04": 77.69, "Te05": 82.62, "Te06": 88.64,
	// ── Efficiency Core 2 (Te0R/S/T triplet) ──────────────────────────────
	"Te0R": 69.32, "Te0S": 73.74, "Te0T": 77.38,
	// ── Efficiency Core 3 (lowercase Tp0l/m/n triplet) ────────────────────
	"Tp0l": 75.68, "Tp0m": 80.01, "Tp0n": 88.34,
	// ── Efficiency Core 4 (non-contiguous Tp0o/q/t triplet) ───────────────
	"Tp0o": 74.83, "Tp0q": 81.16, "Tp0t": 93.52,
	// ── Cluster die aggregates ────────────────────────────────────────────
	"Tpx0": 92.37, "Tpx1": 111.91,
	"Tpx2": 79.77, "Tpx3": 93.52,
	"Tex0": 81.81, "Tex1": 88.64,
	"Tex2": 73.38, "Tex3": 77.38,
}

// a18ProFullReport is the full snapshot from report-a18.txt (lines 815-934),
// including keys that should be filtered out (below minTempCelsius, zero, etc.).
// Used to verify the resolved table matches what a user would see on real hardware.
var a18ProFullReport = map[string]float32{
	// Battery / board
	"TB0T": 28.5, "TBLp": 33.44162,
	// CPU die PMU
	"TCMb": 96.101906, "TCMz": 111.90625,
	// GPU die (ioft) — uppercase T keys
	"TG0A": 28.5, "TG0B": 28.5, "TG0C": 28.399994, "TG0H": 28.399994, "TG0V": 28.399994,
	// Flash/proximity
	"TH0p": 38.076584,
	// Power delivery
	"TPD0": 61.705124, "TPD1": 58.4552, "TPD2": 58.563522, "TPD3": 57.263565,
	"TPD4": 59.32184, "TPD5": 59.10518, "TPD6": 58.130203, "TPDX": 0,
	// ioft die probes — unmapped
	"TQ0d": 43.608704, "TQ0j": 42, "TR0Z": 51.820007, "TR1d": 25.733734, "TR2d": 33.113144,
	// RF proximity / delivery
	"TR5p": 46.506668,
	"TRD0": 34.06299, "TRD1": 33.414993, "TRD2": 33.414993, "TRD3": 34.17099, "TRDX": 0,
	// SSD proximity
	"TS0p": 54.604843, "TS1p": 59.980774,
	// Touch / USB-C / virtual
	"TTSp": 26.082855, "TUCp": 34.857666,
	"TVA0": 20.22112, "TVA1": 21.787788,
	"TVD0": 96.101906, "TVMD": 1, "TVMR": 64.25298,
	"TVS0": 31.172697, "TVS1": 31.886295, "TVS3": 32.129635, "TVS4": 33.311043,
	"TVV0": 0, "TVmS": 64.25298,
	// WiFi proximity
	"TW0p": 41.94023,
	// Ambient (all below minTempCelsius, should be rejected)
	"Ta00": 0, "Ta01": 6.525, "Ta08": 0, "Ta09": 7.325,
	"Ta0C": 0, "Ta0D": 6.725, "Ta0O": 0, "Ta0P": 7.725,
	"Ta0R": 0, "Ta0S": 7.725, "Ta0U": 0, "Ta0V": 7.025,
	// Ambient top — note lowercase p (TaTp), not uppercase P (TaTP)
	"TaTp": 32.75183,
	// E-cores (triplet, M3/M4-style)
	"Te04": 77.69375, "Te05": 82.61875, "Te06": 88.640625,
	"Te0R": 69.31753, "Te0S": 73.74253, "Te0T": 77.375,
	// E-cluster die
	"Tex0": 81.809494, "Tex1": 88.640625, "Tex2": 73.38319, "Tex3": 77.375,
	// GPU (doublets)
	"Tg04": 62.688957, "Tg05": 66.413956,
	"Tg0C": 63.862648, "Tg0D": 68.08765,
	"Tg0K": 62.33503, "Tg0L": 66.56003,
	"Tg0d": 63.28016, "Tg0e": 67.30516,
	// P-cores + cluster aggregates
	"Tp00": 85.63325, "Tp01": 92.75825, "Tp02": 106.296875,
	"Tp04": 87.35511, "Tp05": 94.28011, "Tp06": 110.5625,
	"Tp08": 86.0098, "Tp09": 91.43481, "Tp0A": 106.234375,
	"Tp0C": 88.1769, "Tp0D": 96.101906, "Tp0E": 111.90625,
	// E-core 3 & 4 (lowercase / non-contiguous triplets)
	"Tp0l": 75.68425, "Tp0m": 80.00925, "Tp0n": 88.34375,
	"Tp0o": 74.83265, "Tp0q": 81.157646, "Tp0t": 93.515625,
	// Cluster die aggregates
	"Tpx0": 92.37475, "Tpx1": 111.90625, "Tpx2": 79.76834, "Tpx3": 93.515625,
	// SSDs (triplets)
	"Ts00": 66.147606, "Ts01": 66.147606, "Ts02": 72.828125,
	"Ts04": 67.31417, "Ts05": 67.31417, "Ts06": 74.265625,
	"Ts08": 63.65429, "Ts09": 63.65429, "Ts0A": 68.8125,
	"Ts0C": 63.816048, "Ts0D": 63.816048, "Ts0E": 69.109375,
	"Ts0G": 74.8205, "Ts0H": 74.8205, "Ts0I": 84.5,
	"Ts0K": 72.43232, "Ts0L": 72.43232, "Ts0M": 81.296875,
	"Tsx0": 78.85631, "Tsx1": 84.5,
	// Thermal zones (all zero, rejected)
	"Tz11": 0, "Tz12": 0, "Tz13": 0, "Tz14": 0, "Tz15": 0,
	"Tz16": 0, "Tz17": 0, "Tz18": 0, "Tz1j": 0,
}

// resolveForFamily replays the full SMC resolution pipeline (family filter +
// wildcard expansion + isValidReading + last-write-wins) against a snapshot,
// matching what addGeneric would produce during a live SMC read.
// It returns a map keyed by Desc (as shown to the user) → (key, value).
func resolveForFamily(sensors []SensorStat, family string, snapshot map[string]float32) map[string]struct {
	Key string
	Val float32
} {
	out := make(map[string]struct {
		Key string
		Val float32
	})

	familyApple := strings.HasPrefix(family, "M") || strings.HasPrefix(family, "A") || family == "Apple"

	for _, s := range sensors {
		// Platform gate
		passes := false

		switch {
		case s.Platform == "Apple" && familyApple:
			passes = true
		case s.Platform == "" || s.Platform == "All":
			passes = true
		case s.Platform == family:
			passes = true
		}

		if !passes {
			continue
		}

		// Wildcard expansion (mirrors smc.getGenericSensors)
		if !strings.Contains(s.Key, "%") {
			if v, ok := snapshot[s.Key]; ok && isValidReading(v, TempUnit) {
				out[s.Desc] = struct {
					Key string
					Val float32
				}{Key: s.Key, Val: v}
			}

			continue
		}

		for i := range 10 {
			iKey := strings.Replace(s.Key, "%", fmt.Sprintf("%d", i), 1)
			iDesc := strings.Replace(s.Desc, "%", fmt.Sprintf("%d", i+1), 1)

			if v, ok := snapshot[iKey]; ok && isValidReading(v, TempUnit) {
				out[iDesc] = struct {
					Key string
					Val float32
				}{Key: iKey, Val: v}
			}
		}
	}

	return out
}

// Test_A18ProResolvedTable dumps the full resolved Temperature table as it
// would appear to a user running `iSMC` on Mac17,5 with the A18 report
// snapshot. Run with `go test -run Test_A18ProResolvedTable -v ./smc/` to
// inspect the output. Also asserts invariants: 2 P-cores, 4 E-cores, no
// sensor below minTempCelsius.
func Test_A18ProResolvedTable(t *testing.T) {
	resolved := resolveForFamily(AppleTemp, "A18", a18ProFullReport)

	// Sort descriptions for stable output.
	names := make([]string, 0, len(resolved))
	for name := range resolved {
		names = append(names, name)
	}

	sort.Strings(names)

	t.Logf("===== Resolved Temperature table for Mac17,5 (A18 Pro) =====")
	t.Logf("%-38s  %-6s  %10s", "Sensor", "Key", "Value °C")

	for _, name := range names {
		e := resolved[name]
		t.Logf("%-38s  %-6s  %10.4f", name, e.Key, e.Val)
	}

	t.Logf("===== %d rows resolved =====", len(resolved))

	// Invariants
	var p, e int

	for name := range resolved {
		switch {
		case strings.HasPrefix(name, "CPU Performance Core "):
			p++
		case strings.HasPrefix(name, "CPU Efficiency Core "):
			e++
		}
	}

	assert.Equal(t, 2, p, "expected 2 P-cores")
	assert.Equal(t, 4, e, "expected 4 E-cores")

	for name, entry := range resolved {
		assert.GreaterOrEqual(t, entry.Val, float32(minTempCelsius),
			"%s resolved below minTempCelsius (%g)", name, entry.Val)
	}
}

// Test_A18ProMapping verifies that the A18 temperature sensor definitions,
// combined with the isValidReading filter, resolve the A18 Pro SMC snapshot
// to exactly 2 Performance Cores and 4 Efficiency Cores, with no stray
// additional cores bleeding in from cluster-level aggregates (Tp08-0E).
func Test_A18ProMapping(t *testing.T) {
	a18Sensors := make([]SensorStat, 0, 64)

	for _, s := range AppleTemp {
		if s.Platform == "A18" {
			a18Sensors = append(a18Sensors, s)
		}
	}

	resolved := resolveM4Sensors(a18Sensors, a18ProReport)

	var (
		pCores     int
		eCores     int
		pCoreNames []string
		eCoreNames []string
	)

	for desc := range resolved {
		switch {
		case strings.HasPrefix(desc, "CPU Performance Core "):
			pCores++
			pCoreNames = append(pCoreNames, desc)
		case strings.HasPrefix(desc, "CPU Efficiency Core "):
			eCores++
			eCoreNames = append(eCoreNames, desc)
		}
	}

	assert.Equal(t, 2, pCores,
		"A18 Pro must resolve to exactly 2 Performance Cores; got %v", pCoreNames)
	assert.Equal(t, 4, eCores,
		"A18 Pro must resolve to exactly 4 Efficiency Cores; got %v", eCoreNames)

	// Cluster aggregates must resolve under non-core labels (single P-cluster on A18 Pro).
	_, ca1 := resolved["CPU Performance Cluster Aggregate 1"]
	_, ca2 := resolved["CPU Performance Cluster Aggregate 2"]
	assert.True(t, ca1, "CPU Performance Cluster Aggregate 1 must resolve from Tp08/09/0A")
	assert.True(t, ca2, "CPU Performance Cluster Aggregate 2 must resolve from Tp0C/0D/0E")

	// Guard against phantom P-cores 3+: only 2 P-cores exist on A18 Pro.
	for i := 3; i <= 6; i++ {
		phantom := fmt.Sprintf("CPU Performance Core %d", i)
		_, found := resolved[phantom]
		assert.False(t, found, "phantom %q must not appear (A18 Pro has 2 P-cores)", phantom)
	}
}

// Test_isValidReading verifies the sentinel-rejection and minimum-temperature logic.
func Test_isValidReading(t *testing.T) {
	tests := []struct {
		name string
		val  float32
		unit string
		want bool
	}{
		{"negative sentinel", -4.0, TempUnit, false},
		{"zero", 0.0, TempUnit, false},
		{"near-zero rounds to 0", 0.004, TempUnit, false},
		{"below minTemp", 2.2, TempUnit, false},
		{"below minTemp 3.4", 3.4, TempUnit, false},
		{"below minTemp 5.2", 5.2, TempUnit, false},
		{"exactly minTemp", 10.0, TempUnit, true},
		{"plausible idle", 35.0, TempUnit, true},
		{"plausible load", 85.0, TempUnit, true},
		// Non-temperature units must not apply the minTempCelsius guard.
		{"low voltage", 0.9, "V", true},
		{"small current", 0.5, "A", true},
		{"zero voltage", 0.0, "V", false},
		{"negative voltage", -1.0, "V", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidReading(tt.val, tt.unit)
			assert.Equal(t, tt.want, got,
				"isValidReading(%g, %q) = %v, want %v", tt.val, tt.unit, got, tt.want)
		})
	}
}
