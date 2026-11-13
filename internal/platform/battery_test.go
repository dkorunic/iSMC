// SPDX-FileCopyrightText: Copyright (C) 2026  Dinko Korunic
// SPDX-License-Identifier: GPL-3.0-only

//go:build darwin

package platform

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// props returns a property getter over a fixed AppleSmartBattery snapshot.
func props(m map[string]int64) func(string) (int64, bool) {
	return func(k string) (int64, bool) {
		v, ok := m[k]

		return v, ok
	}
}

func value(t *testing.T, rows map[string]any, desc string) string {
	t.Helper()

	r, ok := rows[desc].(map[string]any)
	if !assert.True(t, ok, "missing row %q", desc) {
		return ""
	}

	s, _ := r["value"].(string)

	return s
}

// A laptop battery (Apple Silicon: CurrentCapacity/MaxCapacity are percent).
func Test_batteryRows_laptop(t *testing.T) {
	t.Parallel()

	rows := batteryRows(props(map[string]int64{
		"BatteryInstalled": 1, "CycleCount": 312, "CurrentCapacity": 87, "MaxCapacity": 100,
		"DesignCapacity": 6075, "NominalChargeCapacity": 5103, "AppleRawMaxCapacity": 5000,
		"Voltage": 12643, "Amperage": 1520, "ExternalConnected": 1, "IsCharging": 1, "FullyCharged": 0,
	}))

	assert.Equal(t, "312", value(t, rows, "Battery Cycle Count"))
	assert.Equal(t, "87 %", value(t, rows, "Battery Charge"))
	assert.Equal(t, "6075 mAh", value(t, rows, "Battery Design Capacity"))
	assert.Equal(t, "5103 mAh", value(t, rows, "Battery Full Charge Capacity"), "NominalChargeCapacity wins")
	assert.Equal(t, "84 %", value(t, rows, "Battery Health"))
	assert.Equal(t, "12.643 V", value(t, rows, "Battery Voltage"))
	assert.Equal(t, "1.520 A", value(t, rows, "Battery Amperage"))
	assert.Equal(t, "Charging", value(t, rows, "Battery State"))
}

// Desktops publish a zero-filled stub with BatteryInstalled false (seen on a
// Mac Studio); no rows may come from it.
func Test_batteryRows_desktopStub(t *testing.T) {
	t.Parallel()

	stub := map[string]int64{
		"BatteryInstalled": 0, "CycleCount": 0, "CurrentCapacity": 0, "MaxCapacity": 0,
		"Voltage": 0, "Amperage": 0, "ExternalConnected": 1, "IsCharging": 0,
	}

	assert.Empty(t, batteryRows(props(stub)))

	delete(stub, "BatteryInstalled")
	assert.Empty(t, batteryRows(props(stub)), "no BatteryInstalled means no battery")
}

// Discharging, older firmware without NominalChargeCapacity.
func Test_batteryRows_onBattery(t *testing.T) {
	t.Parallel()

	rows := batteryRows(props(map[string]int64{
		"BatteryInstalled": 1, "DesignCapacity": 4000, "AppleRawMaxCapacity": 3000,
		"Amperage": -850, "ExternalConnected": 0,
	}))

	assert.Equal(t, "3000 mAh", value(t, rows, "Battery Full Charge Capacity"))
	assert.Equal(t, "75 %", value(t, rows, "Battery Health"))
	assert.Equal(t, "-0.850 A", value(t, rows, "Battery Amperage"))
	assert.Equal(t, "On Battery", value(t, rows, "Battery State"))
	assert.NotContains(t, rows, "Battery Charge", "no MaxCapacity, no charge level")
}
