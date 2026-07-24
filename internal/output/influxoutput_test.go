// SPDX-FileCopyrightText: Copyright (C) 2023 Seaburr
// SPDX-License-Identifier: GPL-3.0-only

//go:build darwin

package output

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// stripTimestamp removes the trailing nanosecond timestamp from an InfluxDB line
// protocol line, leaving only the measurement+tags and field set portions.
func stripTimestamp(line string) string {
	line = strings.TrimRight(line, "\n")
	idx := strings.LastIndex(line, " ")

	if idx < 0 {
		return line
	}

	return line[:idx]
}

func Test_influxStringConvert(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"spaces to underscores", "CPU Temperature", "cpu_temperature"},
		{"already lowercase", "fan", "fan"},
		{"uppercase", "BATTERY", "battery"},
		{"multiple spaces", "GPU Core 1", "gpu_core_1"},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, influxStringConvert(tt.input))
		})
	}
}

func Test_influxFieldValue(t *testing.T) {
	tests := []struct {
		name      string
		input     any
		wantField string
		wantUnit  string
	}{
		{"temperature with degree unit", "25.5 °C", "25.5", "c"},
		{"ampere unit", "1.2 A", "1.2", "a"},
		{"volt unit", "5.0 V", "5.0", "v"},
		{"rpm with width-padded leading space", "  800 rpm", "800", "rpm"},
		{"numeric no unit", "42", "42", "none"},
		{"bare integer value", uint32(4), "4", "none"},
		{"boolean stays unquoted", false, "false", "none"},
		{"free-text cpu quoted, not split into unit", "M1 Ultra", `"M1 Ultra"`, "none"},
		{"free-text with comma quoted so it cannot corrupt the line", "Mac13,2", `"Mac13,2"`, "none"},
		{"free-text with spaces and parens quoted whole", "Mac Studio (M1 Ultra)", `"Mac Studio (M1 Ultra)"`, "none"},
		{"embedded quote escaped", `a"b`, `"a\"b"`, "none"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			field, unit := influxFieldValue(tt.input)
			assert.Equal(t, tt.wantField, field, "field")
			assert.Equal(t, tt.wantUnit, unit, "unit")
		})
	}
}

// TestInfluxOutput_stringField verifies that free-text hardware values (which
// contain spaces and commas) are emitted as quoted string fields rather than
// unquoted tokens that truncate or split the line-protocol record.
func TestInfluxOutput_stringField(t *testing.T) {
	var out bytes.Buffer

	GetHardware = func() map[string]any {
		return map[string]any{
			"Model": map[string]any{
				"key":   "hw.model",
				"value": "Mac Studio (M1 Ultra)",
				"type":  "hid",
			},
		}
	}

	o := InfluxOutput{writer: io.Writer(&out)}
	o.Hardware()

	line := stripTimestamp(out.String())
	assert.Equal(t,
		`model,sensortype=hardware,unit=none,key=hw.model value="Mac Studio (M1 Ultra)"`,
		line)
}

func TestInfluxOutput_methods(t *testing.T) {
	sensor := map[string]any{
		"sensor": map[string]any{
			"key":   "TC0H",
			"value": "25.000000 °C",
			"type":  "sp78",
		},
	}

	tests := []struct {
		name        string
		monkeyPatch func()
		method      func(io InfluxOutput)
		wantPrefix  string
	}{
		{
			"Battery",
			func() { GetBattery = func() map[string]any { return sensor } },
			func(io InfluxOutput) { io.Battery() },
			"sensor,sensortype=battery,unit=c,key=tc0h value=25.000000",
		},
		{
			"Current",
			func() { GetCurrent = func() map[string]any { return sensor } },
			func(io InfluxOutput) { io.Current() },
			"sensor,sensortype=current,unit=c,key=tc0h value=25.000000",
		},
		{
			"Fans",
			func() { GetFans = func() map[string]any { return sensor } },
			func(io InfluxOutput) { io.Fans() },
			"sensor,sensortype=fans,unit=c,key=tc0h value=25.000000",
		},
		{
			"Power",
			func() { GetPower = func() map[string]any { return sensor } },
			func(io InfluxOutput) { io.Power() },
			"sensor,sensortype=power,unit=c,key=tc0h value=25.000000",
		},
		{
			"Temperature",
			func() { GetTemperature = func() map[string]any { return sensor } },
			func(io InfluxOutput) { io.Temperature() },
			"sensor,sensortype=temperature,unit=c,key=tc0h value=25.000000",
		},
		{
			"Voltage",
			func() { GetVoltage = func() map[string]any { return sensor } },
			func(io InfluxOutput) { io.Voltage() },
			"sensor,sensortype=voltage,unit=c,key=tc0h value=25.000000",
		},
	}

	for _, tt := range tests {
		var out bytes.Buffer

		t.Run(tt.name, func(t *testing.T) {
			tt.monkeyPatch()

			o := InfluxOutput{writer: io.Writer(&out)}
			tt.method(o)

			line := stripTimestamp(out.String())
			assert.Equal(t, tt.wantPrefix, line)
		})
	}
}

func TestInfluxOutput_All(t *testing.T) {
	var out bytes.Buffer

	GetAll = func() map[string]any {
		return map[string]any{
			"Temperature": map[string]any{
				"CPU": map[string]any{
					"key":   "TC0H",
					"value": "50.000000 °C",
					"type":  "sp78",
				},
			},
		}
	}

	o := InfluxOutput{writer: io.Writer(&out)}
	o.All()

	line := stripTimestamp(out.String())
	assert.Equal(t, "cpu,sensortype=temperature,unit=c,key=tc0h value=50.000000", line)
}

// TestInfluxOutput_emptyKey verifies TC-18: sensors whose "key" field is an empty
// string must not emit a ",key=" fragment in the InfluxDB line protocol tag set.
// A malformed ",key=" tag is invalid InfluxDB syntax and would be rejected at ingest.
func TestInfluxOutput_emptyKey(t *testing.T) {
	var out bytes.Buffer

	GetTemperature = func() map[string]any {
		return map[string]any{
			"CPU Temperature": map[string]any{
				"key":   "",
				"value": "50.000000 °C",
				"type":  "hid",
			},
		}
	}

	o := InfluxOutput{writer: io.Writer(&out)}
	o.Temperature()

	line := out.String()
	assert.NotContains(t, line, ",key=",
		"empty sensor key must not produce a ,key= fragment in InfluxDB output")
	assert.Contains(t, line, "cpu_temperature,sensortype=temperature",
		"measurement and sensortype tag must still be present")
}

// TestInfluxOutput_unitExtraction verifies TC-16: the unit tag must contain the
// unit symbol (e.g. "c" for Celsius), NOT the numeric value. The bug under test
// uses Split(...)[0] instead of [1], producing unit=50.000000 instead of unit=c.
func TestInfluxOutput_unitExtraction(t *testing.T) {
	var out bytes.Buffer

	GetTemperature = func() map[string]any {
		return map[string]any{
			"CPU Temp": map[string]any{
				"key":   "TC0H",
				"value": "50.000000 °C",
				"type":  "sp78",
			},
		}
	}

	o := InfluxOutput{writer: io.Writer(&out)}
	o.Temperature()

	line := out.String()
	assert.Contains(t, line, "unit=c",
		"unit tag must be the unit symbol 'c', not the numeric value")
	assert.NotContains(t, line, "unit=50",
		"unit tag must NOT contain the numeric sensor value")
}

func TestInfluxOutput_empty(t *testing.T) {
	var out bytes.Buffer

	GetBattery = func() map[string]any { return map[string]any{} }

	o := InfluxOutput{writer: io.Writer(&out)}
	o.Battery()

	assert.Empty(t, out.String(), "empty sensor map should produce no output")
}
