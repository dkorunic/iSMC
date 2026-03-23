// Copyright (C) 2022 Roland Schaer
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

package output

import (
	"bytes"
	"encoding/json"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Test_format_sp78 verifies TC-19: sensors of type "sp78" must be enriched with
// numeric "quantity" and string "unit" fields by the format function. If "sp78" were
// absent from the switch-case list, those fields would be silently omitted.
func Test_format_sp78(t *testing.T) {
	input := map[string]any{
		"CPU Temp": map[string]any{
			"key":   "TC0H",
			"value": "25.5 °C",
			"type":  "sp78",
		},
	}

	result, err := format(input)
	assert.NoError(t, err)

	resultMap, ok := result.(map[string]any)
	assert.True(t, ok)

	// JSON-round-trip the entry to get a plain map for field inspection
	raw, _ := json.Marshal(resultMap["CPU Temp"])
	var entry map[string]any
	assert.NoError(t, json.Unmarshal(raw, &entry))

	assert.Equal(t, 25.5, entry["quantity"],
		"sp78 sensor must have a numeric 'quantity' field after format()")
	assert.Equal(t, "°C", entry["unit"],
		"sp78 sensor must have a 'unit' field after format()")
}

// Test_format_nonStringValue verifies TC-21: sensors whose "value" field is not a
// string (e.g. bool for flag/battery sensors, uint32 for counts) must not cause
// format() to return an error. The default branch must skip such entries silently.
func Test_format_nonStringValue(t *testing.T) {
	tests := []struct {
		name  string
		value any
		typ   string
	}{
		{"bool flag", true, "flag"},
		{"uint battery count", uint32(1), "ui8"},
		{"int zero", 0, "ui8"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := map[string]any{
				"Battery Power": map[string]any{
					"key":   "BATP",
					"value": tt.value,
					"type":  tt.typ,
				},
			}

			_, err := format(input)
			assert.NoError(t, err,
				"format() must not fail for non-string value type %T", tt.value)
		})
	}
}

func TestJSONOutput(t *testing.T) {
	tests := []struct {
		name        string
		monkeyPatch func()
		method      func(jo JSONOutput)
		expected    string
	}{
		{
			"All sensors",
			func() {
				GetAll = func() map[string]any {
					return map[string]any{
						"sensor-1": map[string]any{
							"key":   "key",
							"value": "string",
							"type":  "type",
						},
						"sensor-2": map[string]any{
							"key":   "key",
							"value": true,
							"type":  "type",
						},
						"sensor-3": map[string]any{
							"key":   "key",
							"value": 99,
							"type":  "type",
						},
					}
				}
			},
			func(jo JSONOutput) {
				jo.All()
			},
			`{"sensor-1":{"key":"key","value":"string","type":"type"},"sensor-2":{"key":"key","value":true,"type":"type"},"sensor-3":{"key":"key","value":99,"type":"type"}}`,
		},
		{
			"Battery sensor",
			func() {
				GetBattery = func() map[string]any {
					return getMapForSensor("battery")
				}
			},
			func(jo JSONOutput) {
				jo.Battery()
			},
			`{"battery":{"key":"key","value":"value","type":"type"}}`,
		},
		{
			"Current sensor",
			func() {
				GetCurrent = func() map[string]any {
					return getMapForSensor("current")
				}
			},
			func(jo JSONOutput) {
				jo.Current()
			},
			`{"current":{"key":"key","value":"value","type":"type"}}`,
		},
		{
			"Fans sensor",
			func() {
				GetFans = func() map[string]any {
					return getMapForSensor("fans")
				}
			},
			func(jo JSONOutput) {
				jo.Fans()
			},
			`{"fans":{"key":"key","value":"value","type":"type"}}`,
		},
		{
			"Power sensor",
			func() {
				GetPower = func() map[string]any {
					return getMapForSensor("power")
				}
			},
			func(jo JSONOutput) {
				jo.Power()
			},
			`{"power":{"key":"key","value":"value","type":"type"}}`,
		},
		{
			"Temperature sensor",
			func() {
				GetTemperature = func() map[string]any {
					return getMapForSensor("temperature")
				}
			},
			func(jo JSONOutput) {
				jo.Temperature()
			},
			`{"temperature":{"key":"key","value":"value","type":"type"}}`,
		},
		{
			"Voltage sensor",
			func() {
				GetVoltage = func() map[string]any {
					return getMapForSensor("voltage")
				}
			},
			func(jo JSONOutput) {
				jo.Voltage()
			},
			`{"voltage":{"key":"key","value":"value","type":"type"}}`,
		},
	}
	for _, tt := range tests {
		var out bytes.Buffer

		t.Run(tt.name, func(t *testing.T) {
			tt.monkeyPatch()

			jo := JSONOutput{writer: io.Writer(&out)}
			tt.method(jo)

			actual := out.String()
			assert.JSONEq(t, tt.expected, actual)
		})
	}
}
