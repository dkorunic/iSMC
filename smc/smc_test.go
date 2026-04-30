// Copyright (C) 2019  Dinko Korunic
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
	"runtime"
	"strings"
	"testing"

	"github.com/dkorunic/iSMC/platform"
	"github.com/stretchr/testify/assert"
)

func Test_filterForPlatform(t *testing.T) {
	allSensor := SensorStat{Key: "TALL", Desc: "All sensor", Platform: "All"}
	emptySensor := SensorStat{Key: "TEMP", Desc: "Empty platform sensor", Platform: ""}
	appleSensor := SensorStat{Key: "TAPL", Desc: "Apple Silicon sensor", Platform: "Apple"}
	intelSensor := SensorStat{Key: "TINT", Desc: "Intel-only sensor", Platform: "Intel"}
	m1Sensor := SensorStat{Key: "TM1S", Desc: "M1-only sensor", Platform: "M1"}

	input := []SensorStat{allSensor, emptySensor, appleSensor, intelSensor, m1Sensor}
	result := filterForPlatform(input)

	// "All" and "" sensors are always included regardless of platform.
	assert.Contains(t, result, allSensor, "sensor with Platform=All must always be included")
	assert.Contains(t, result, emptySensor, "sensor with Platform='' must always be included")
}

func Test_filterForPlatform_empty(t *testing.T) {
	result := filterForPlatform([]SensorStat{})
	assert.Empty(t, result)
}

func Test_filterForPlatform_allOnly(t *testing.T) {
	sensors := []SensorStat{
		{Key: "T1", Desc: "Sensor 1", Platform: "All"},
		{Key: "T2", Desc: "Sensor 2", Platform: "All"},
	}

	result := filterForPlatform(sensors)
	assert.Len(t, result, 2)
}

// Test_filterForPlatform_appleSiliconInclusion verifies TC-11: that sensors tagged
// Platform="Apple" are included on Apple Silicon hardware. The bug under test replaces
// strings.HasPrefix with strings.HasSuffix, which would never match "M1", "M2", "A18"
// etc. (those strings end in digits, not "M" or "A").
//
// This test is conditional: it only asserts Apple-specific inclusion when the actual
// hardware reports an M- or A-family chip.
func Test_filterForPlatform_appleSiliconInclusion(t *testing.T) {
	appleSensor := SensorStat{Key: "TAPL", Desc: "Apple Silicon sensor", Platform: "Apple"}
	intelSensor := SensorStat{Key: "TINT", Desc: "Intel-only sensor", Platform: "Intel"}
	input := []SensorStat{appleSensor, intelSensor}

	result := filterForPlatform(input)

	family := platform.GetFamily()
	if family == "" || family == "Unknown" {
		switch runtime.GOARCH {
		case "arm64":
			family = "Apple"
		case "amd64", "386":
			family = "Intel"
		}
	}

	isAppleSilicon := strings.HasPrefix(family, "M") || strings.HasPrefix(family, "A") || family == "Apple"
	if isAppleSilicon {
		assert.Contains(t, result, appleSensor, "Apple sensor must be included on Apple Silicon (family=%q)", family)
		assert.NotContains(t, result, intelSensor, "Intel sensor must NOT be included on Apple Silicon (family=%q)", family)
	} else {
		assert.NotContains(t, result, appleSensor, "Apple sensor must NOT be included on Intel (family=%q)", family)
		assert.Contains(t, result, intelSensor, "Intel sensor must be included on Intel (family=%q)", family)
	}
}

// filterByFamily is a pure-function mirror of the inner loop of filterForPlatform
// that takes `family` as an explicit parameter. This enables deterministic tests
// across family values (A18, M1, etc.) without depending on the host's sysctl
// output. Any change here MUST be mirrored in filterForPlatform.
func filterByFamily(smcSlice []SensorStat, family string) []SensorStat {
	filteredSensors := make([]SensorStat, 0, len(smcSlice))

	familyApple := strings.HasPrefix(family, "M") || strings.HasPrefix(family, "A") || family == "Apple"

	for _, v := range smcSlice {
		if v.Platform == "Apple" && familyApple {
			filteredSensors = append(filteredSensors, v)

			continue
		}

		if v.Platform == "" || v.Platform == "All" {
			filteredSensors = append(filteredSensors, v)

			continue
		}

		if v.Platform == family {
			filteredSensors = append(filteredSensors, v)

			continue
		}
	}

	return filteredSensors
}

// Test_filterByFamily_A18 verifies the platform-filter contract for the Mac17,5
// (A18 Pro) family: All + Apple + A18-tagged rows are admitted, every other
// family-specific row (M1-M5, Intel) is rejected. Runs deterministically
// regardless of the host hardware.
func Test_filterByFamily_A18(t *testing.T) {
	input := []SensorStat{
		{Key: "TALL", Desc: "All", Platform: "All"},
		{Key: "TEMP", Desc: "Empty", Platform: ""},
		{Key: "TAPL", Desc: "Apple", Platform: "Apple"},
		{Key: "TA18", Desc: "A18", Platform: "A18"},
		{Key: "TM1S", Desc: "M1", Platform: "M1"},
		{Key: "TM4S", Desc: "M4", Platform: "M4"},
		{Key: "TM5S", Desc: "M5", Platform: "M5"},
		{Key: "TINT", Desc: "Intel", Platform: "Intel"},
	}

	result := filterByFamily(input, "A18")

	admitted := make(map[string]bool, len(result))
	for _, s := range result {
		admitted[s.Desc] = true
	}

	// Must be admitted
	for _, want := range []string{"All", "Empty", "Apple", "A18"} {
		assert.True(t, admitted[want], "family=A18 must admit Platform=%q", want)
	}

	// Must be rejected
	for _, reject := range []string{"M1", "M4", "M5", "Intel"} {
		assert.False(t, admitted[reject], "family=A18 must reject Platform=%q", reject)
	}

	assert.Len(t, result, 4, "family=A18 must admit exactly 4 of the 8 test rows")
}

// Test_filterByFamily_A18_GeneratedTable exercises the actual AppleTemp slice
// from the generated sensors.go with family="A18" and asserts that the core
// A18 CPU sensors resolve through the filter. This guards against a regression
// where the A18 rows are stripped from temp.txt or the filter rule changes.
func Test_filterByFamily_A18_GeneratedTable(t *testing.T) {
	result := filterByFamily(AppleTemp, "A18")

	// Build a lookup of admitted keys by Platform tag.
	byPlatform := make(map[string]int)
	keys := make(map[string]string) // key -> platform

	for _, s := range result {
		byPlatform[s.Platform]++
		keys[s.Key] = s.Platform
	}

	// Must include at least the canonical A18 P-core/E-core keys.
	for _, mustHave := range []string{
		"Tp00", "Tp01", "Tp02", // P-core 1 triplet
		"Tp04", "Tp05", "Tp06", // P-core 2 triplet
		"Te04", "Te05", "Te06", // E-core 1 triplet
		"Te0R", "Te0S", "Te0T", // E-core 2 triplet
		"Tp0l", "Tp0m", "Tp0n", // E-core 3 triplet
		"Tp0o", "Tp0q", "Tp0t", // E-core 4 triplet
	} {
		if plat, ok := keys[mustHave]; !ok {
			t.Errorf("A18 key %q not admitted through filterByFamily", mustHave)
		} else if plat != "A18" {
			t.Errorf("A18 key %q admitted with Platform=%q, want A18", mustHave, plat)
		}
	}

	// Must NOT include any M-family- or Intel-only rows.
	for _, s := range result {
		switch s.Platform {
		case "M1", "M2", "M3", "M4", "M5", "Intel":
			t.Errorf("family=A18 leaked Platform=%q row (key=%q desc=%q)",
				s.Platform, s.Key, s.Desc)
		}
	}

	// Sanity: A18 contributes 69 A18-exclusive rows after consolidating Apple-wide
	// keys into wildcards (TVA%, TVS%, TPD%, TRD%) and promoting TVMR/TVmS to Apple.
	assert.Equal(t, 69, byPlatform["A18"],
		"expected 69 A18-tagged rows in AppleTemp; got %d", byPlatform["A18"])
}

// Test_filterForPlatform_exactFamilyMatch verifies that platform-specific sensors are
// only included when the family string matches exactly (TC-11 supporting test).
func Test_filterForPlatform_exactFamilyMatch(t *testing.T) {
	// "M1"-tagged sensor should only appear if the machine is an M1.
	// On all other hardware (M2, M3, Intel, etc.) it must be excluded.
	m1Sensor := SensorStat{Key: "TM1S", Desc: "M1-only sensor", Platform: "M1"}
	m2Sensor := SensorStat{Key: "TM2S", Desc: "M2-only sensor", Platform: "M2"}
	input := []SensorStat{m1Sensor, m2Sensor}

	result := filterForPlatform(input)

	family := platform.GetFamily()
	if family == "" || family == "Unknown" {
		// Can't make assertions without a known family; skip
		t.Skip("platform family unknown, skipping exact-match test")
	}

	switch family {
	case "M1":
		assert.Contains(t, result, m1Sensor)
		assert.NotContains(t, result, m2Sensor)
	case "M2":
		assert.NotContains(t, result, m1Sensor)
		assert.Contains(t, result, m2Sensor)
	default:
		assert.NotContains(t, result, m1Sensor, "M1 sensor must not appear on family=%q", family)
		assert.NotContains(t, result, m2Sensor, "M2 sensor must not appear on family=%q", family)
	}
}

// Test_filterByFamily_NoPCIeLeakOnAppleSilicon guards against a regression where
// the Intel-era PCIe slot wildcards (Te%F/P/S/T) are re-tagged Platform="All"
// and start leaking phantom "PCIe Slot N Side/Bottom" labels onto Apple Silicon
// families. On Apple Silicon (M*/A*) Te0S and Te0T are reassigned as E-core 2
// triplet probes, so any PCIe label referencing those keys is wrong.
//
// If this test fails, check src/temp.txt lines for "PCIe Slot %" and confirm
// they are Platform: Intel, not All.
func Test_filterByFamily_NoPCIeLeakOnAppleSilicon(t *testing.T) {
	for _, family := range []string{"M1", "M2", "M3", "M4", "M5", "A18", "Apple"} {
		result := filterByFamily(AppleTemp, family)
		for _, s := range result {
			if strings.HasPrefix(s.Desc, "PCIe Slot ") {
				t.Errorf("family=%q leaked PCIe label: Key=%q Desc=%q Platform=%q",
					family, s.Key, s.Desc, s.Platform)
			}
		}
	}
}

func Test_platformMatches(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		rowPlatform  string
		family       string
		wantMatching bool
	}{
		{"empty row matches all", "", "M5", true},
		{"All row matches all", "All", "Intel", true},
		{"Apple row matches M-family", "Apple", "M1", true},
		{"Apple row matches A-family", "Apple", "A18", true},
		{"Apple row matches generic Apple", "Apple", "Apple", true},
		{"Apple row rejects Intel", "Apple", "Intel", false},
		{"Exact M5 match", "M5", "M5", true},
		{"M3 row rejects M5 family", "M3", "M5", false},
		{"Intel row rejects Apple Silicon", "Intel", "M1", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := platformMatches(tt.rowPlatform, tt.family)
			if got != tt.wantMatching {
				t.Errorf("platformMatches(%q, %q) = %v, want %v",
					tt.rowPlatform, tt.family, got, tt.wantMatching)
			}
		})
	}
}

// Test_LookupTempDesc_directMatch covers the three matching paths in
// LookupTempDesc: direct (no-wildcard) keys, wildcard expansion, and
// platform-scoped rejection. Sentinel: a key tagged Platform="M1" must not
// resolve when queried under a different family.
func Test_LookupTempDesc_directMatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		key      string
		family   string
		wantDesc string
		wantOK   bool
	}{
		{
			name: "M5 base Tp00 maps to Super Core 1",
			// AppleTemp has explicit M5 rows for Tp00/Tp04/Tp08/Tp0C as Super Core 1..4.
			key: "Tp00", family: "M5",
			wantDesc: "CPU Super Core 1", wantOK: true,
		},
		{
			name: "M3 Tp1E maps to Performance Core 7",
			key:  "Tp1E", family: "M3",
			wantDesc: "CPU Performance Core 7", wantOK: true,
		},
		{
			name: "Wildcard TC%c expands for digit indices",
			key:  "TC0c", family: "Intel",
			wantDesc: "CPU Core 1", wantOK: true,
		},
		{
			name: "Apple-tagged keys match any M-family",
			key:  "TaLP", family: "M4",
			wantDesc: "Airflow Left", wantOK: true,
		},
		{
			name:   "Unknown key returns false",
			key:    "ZZZZ",
			family: "M5",
			wantOK: false,
		},
		{
			name: "M1-only key rejected on M3",
			// Tp08 maps to "CPU Efficiency Core 1" only on M1 in AppleTemp.
			key: "Tp08", family: "M3",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			desc, ok := LookupTempDesc(tt.key, tt.family)
			if ok != tt.wantOK {
				t.Errorf("LookupTempDesc(%q, %q) ok = %v, want %v",
					tt.key, tt.family, ok, tt.wantOK)
			}

			if ok && desc != tt.wantDesc {
				t.Errorf("LookupTempDesc(%q, %q) desc = %q, want %q",
					tt.key, tt.family, desc, tt.wantDesc)
			}
		})
	}
}

// Test_MappedTempKeys_M5 exercises the wildcard expansion path: every Tp0?
// slot from AppleTemp's M5 rows must appear as a concrete key in the result.
func Test_MappedTempKeys_M5(t *testing.T) {
	t.Parallel()

	got := MappedTempKeys("M5")

	// Spot-check M5 super cores (explicit AppleTemp rows).
	for _, k := range []string{"Tp00", "Tp04", "Tp08", "Tp0C"} {
		if _, ok := got[k]; !ok {
			t.Errorf("MappedTempKeys(M5) missing %q", k)
		}
	}

	// Wildcard expansion: TC%c → TC0c..TC9c, all should be present.
	for i := range 10 {
		want := "TC" + string(rune('0'+i)) + "c"
		if _, ok := got[want]; !ok {
			t.Errorf("MappedTempKeys(M5) missing wildcard-expanded key %q", want)
		}
	}

	// Apple universal rows must come through (e.g. SSD Proximity 1).
	if desc, ok := got["TS0P"]; !ok || desc != "SSD Proximity 1" {
		t.Errorf("MappedTempKeys(M5)[TS0P] = (%q, %v), want (\"SSD Proximity 1\", true)",
			desc, ok)
	}
}

// Test_MappedTempKeys_familyIsolation makes sure family-scoped rows do not leak
// across families. Tp08 is M1-specific (Efficiency Core 1); querying M1 must
// return it while querying other families must not.
func Test_MappedTempKeys_familyIsolation(t *testing.T) {
	t.Parallel()

	m1 := MappedTempKeys("M1")
	m3 := MappedTempKeys("M3")

	if d, ok := m1["Tp08"]; !ok || d != "CPU Efficiency Core 1" {
		t.Errorf("MappedTempKeys(M1)[Tp08] = (%q, %v), want (\"CPU Efficiency Core 1\", true)", d, ok)
	}

	// Tp1E is M3-specific (Performance Core 7); should not appear on M1.
	if _, ok := m1["Tp1E"]; ok {
		t.Errorf("MappedTempKeys(M1) leaked M3-only key Tp1E")
	}

	if d, ok := m3["Tp1E"]; !ok || d != "CPU Performance Core 7" {
		t.Errorf("MappedTempKeys(M3)[Tp1E] = (%q, %v), want (\"CPU Performance Core 7\", true)", d, ok)
	}
}
