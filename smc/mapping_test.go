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
		phantom := "CPU Performance Core " + string(rune('0'+i))
		if i >= 10 {
			phantom = "CPU Performance Core " + strings.Join([]string{
				string(rune('0' + i/10)), string(rune('0' + i%10)),
			}, "")
		}

		_, found := resolved[phantom]
		assert.False(t, found, "phantom %q must not appear in resolved sensors", phantom)
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
