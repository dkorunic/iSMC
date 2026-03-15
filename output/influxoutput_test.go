// Copyright (C) 2023 Seaburr
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

func Test_influxGetValue(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"value with unit", "25.5 °C", "25.5"},
		{"value only", "42", "42"},
		{"value=prefix form", "value=25.5 °C", "value=25.5"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, influxGetValue(tt.input))
		})
	}
}

func Test_influxGetUnit(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"temperature unit", "value=25.5 °C", "c"},
		{"ampere unit", "value=1.2 A", "a"},
		{"volt unit", "value=5.0 V", "v"},
		{"no unit", "value=42", "none"},
		{"degree stripped", "value=30.0 °C", "c"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, influxGetUnit(tt.input))
		})
	}
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

func TestInfluxOutput_empty(t *testing.T) {
	var out bytes.Buffer

	GetBattery = func() map[string]any { return map[string]any{} }

	o := InfluxOutput{writer: io.Writer(&out)}
	o.Battery()

	assert.Empty(t, out.String(), "empty sensor map should produce no output")
}
