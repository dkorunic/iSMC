// SPDX-FileCopyrightText: Copyright (C) 2026  Dinko Korunic
// SPDX-License-Identifier: GPL-3.0-only

//go:build darwin

package smc

import "testing"

// TestKeepReading: a parked 40.0 °C slot must not overwrite a live reading of
// the same row (alternating M4 Pro/Max P-core banks, iSMC#28), but may fill an
// empty row and must not block a later live reading.
func TestKeepReading(t *testing.T) {
	t.Parallel()

	const desc = "CPU Performance Core 1"

	live := map[string]any{desc: map[string]any{"key": "Tp00", "value": "75 °C"}}
	if keepReading(live, desc, GatedTempCelsius, TempUnit) {
		t.Error("parked 40.0 °C overwrote a live reading")
	}

	empty := map[string]any{}
	if !keepReading(empty, desc, GatedTempCelsius, TempUnit) {
		t.Error("parked 40.0 °C rejected for an empty row")
	}

	if !keepReading(live, desc, 75.5, TempUnit) {
		t.Error("live reading rejected")
	}

	if keepReading(empty, desc, 5, TempUnit) {
		t.Error("sentinel below minTempCelsius accepted")
	}

	// Only temperatures are gated: 40.0 W is a real power reading.
	if !keepReading(live, "Power", GatedTempCelsius, "W") {
		t.Error("40.0 non-temperature reading rejected")
	}
}
