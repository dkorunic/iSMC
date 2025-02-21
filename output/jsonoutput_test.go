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
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
)

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
