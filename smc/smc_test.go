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
	"testing"

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
