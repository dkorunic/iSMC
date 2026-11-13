// SPDX-FileCopyrightText: Copyright (C) 2022 Roland Schaer
// SPDX-License-Identifier: GPL-3.0-only

//go:build darwin

package output

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// asciiTpl is one ASCII section with a single sensor row; %s is the
// centred title line.
var asciiTpl = `%s
+-------------+-----+-------+------+
| Description | Key | Value | Type |
+-------------+-----+-------+------+
| sensor      | key | value | type |
+-------------+-----+-------+------+

`

func TestTableOutput_ASCII(t *testing.T) {
	tests := []struct {
		name        string
		monkeyPatch func()
		method      func(to TableOutput)
		expected    string
	}{
		{
			"All sensors",
			func() {
				GetAll = func() map[string]any {
					return map[string]any{
						"battery": map[string]any{
							"sensor": map[string]any{
								"key":   "key",
								"value": "value",
								"type":  "type",
							},
						},
						"fans": map[string]any{
							"sensor": map[string]any{
								"key":   "key",
								"value": "value",
								"type":  "type",
							},
						},
						"temperature": map[string]any{
							"sensor": map[string]any{
								"key":   "key",
								"value": "value",
								"type":  "type",
							},
						},
					}
				}
			},
			func(to TableOutput) {
				to.All()
			},
			getASCIITpl("battery", "fans", "temperature"),
		},
		{
			"Battery sensor",
			func() {
				GetBattery = func() map[string]any {
					return getMapForSensor("sensor")
				}
			},
			func(to TableOutput) {
				to.Battery()
			},
			getASCIITpl("Battery"),
		},
		{
			"Current sensor",
			func() {
				GetCurrent = func() map[string]any {
					return getMapForSensor("sensor")
				}
			},
			func(to TableOutput) {
				to.Current()
			},
			getASCIITpl("Current"),
		},
		{
			"Fans sensor",
			func() {
				GetFans = func() map[string]any {
					return getMapForSensor("sensor")
				}
			},
			func(to TableOutput) {
				to.Fans()
			},
			getASCIITpl("Fans"),
		},
		{
			"Power sensor",
			func() {
				GetPower = func() map[string]any {
					return getMapForSensor("sensor")
				}
			},
			func(to TableOutput) {
				to.Power()
			},
			getASCIITpl("Power"),
		},
		{
			"Temperature sensor",
			func() {
				GetTemperature = func() map[string]any {
					return getMapForSensor("sensor")
				}
			},
			func(to TableOutput) {
				to.Temperature()
			},
			getASCIITpl("Temperature"),
		},
		{
			"Voltage sensor",
			func() {
				GetVoltage = func() map[string]any {
					return getMapForSensor("sensor")
				}
			},
			func(to TableOutput) {
				to.Voltage()
			},
			getASCIITpl("Voltage"),
		},
	}

	for _, tt := range tests {
		var out bytes.Buffer

		t.Run(tt.name, func(t *testing.T) {
			tt.monkeyPatch()

			to := TableOutput{isASCII: true, writer: io.Writer(&out)}
			tt.method(to)

			actual := out.String()
			assert.Equal(t, tt.expected, actual)
		})
	}
}

// getASCIITpl returns the expected ASCII table output string for the given section titles,
// used to build golden strings in table-driven tests.
func getASCIITpl(title ...string) string {
	const width = 36 // rendered table width

	var out strings.Builder

	for _, t := range title {
		fmt.Fprintf(&out, asciiTpl, strings.Repeat(" ", (width-len(t))/2)+t)
	}

	return out.String()
}
