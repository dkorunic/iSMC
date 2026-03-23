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
