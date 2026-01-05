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
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

var asciiTpl = `+-------------------------------------+
|%s|
+-------------+-----+----------+------+
| DESCRIPTION | KEY | VALUE    | TYPE |
+-------------+-----+----------+------+
| sensor      | key |    value | type |
+-------------+-----+----------+------+

`

//lint:ignore ST1018 stick to unicode characters for test output
var tableTpl = `[96;100;1m%s[0m
[106;30m DESCRIPTION [0m[106;30m KEY [0m[106;30m VALUE    [0m[106;30m TYPE [0m
[107;30m sensor      [0m[107;30m key [0m[107;30m    value [0m[107;30m type [0m

` //nolint:stylecheck

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

func TestTableOutput_Table(t *testing.T) {
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
					}
				}
			},
			func(to TableOutput) {
				to.All()
			},
			getTableTpl("battery", "fans"),
		},
	}
	for _, tt := range tests {
		var out bytes.Buffer

		t.Run(tt.name, func(t *testing.T) {
			tt.monkeyPatch()

			to := TableOutput{isASCII: false, writer: io.Writer(&out)}
			tt.method(to)

			actual := out.String()
			assert.Equal(t, tt.expected, actual)
		})
	}
}

func getASCIITpl(title ...string) string {
	var out strings.Builder

	for _, t := range title {
		// center title
		width := 37
		even := 0

		if len(t)%2 == 0 {
			even = 1
		}

		centeredTitle := fmt.Sprintf("%*s", -width, fmt.Sprintf("%*s", (width+len(t)+even)/2, t))

		out.WriteString(fmt.Sprintf(asciiTpl, centeredTitle))
	}

	return out.String()
}

func getTableTpl(title ...string) string {
	var out strings.Builder

	for _, t := range title {
		// center title
		width := 35
		centeredTitle := fmt.Sprintf("%*s", -width+1, fmt.Sprintf("%*s", (width+len(t))/2, t))

		out.WriteString(fmt.Sprintf(tableTpl, centeredTitle))
	}

	return out.String()
}
