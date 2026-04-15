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
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

type InfluxOutput struct {
	writer io.Writer
}

// NewInfluxOutput returns an InfluxOutput that writes to stdout.
func NewInfluxOutput() Output {
	o := InfluxOutput{}
	o.writer = io.Writer(os.Stdout)

	return o
}

func (io InfluxOutput) All() {
	all := GetAll()

	for _, key := range sortedKeys(all) {
		value := all[key]
		if smcdata, ok := value.(map[string]any); ok {
			io.print(key, smcdata)
		}
	}
}

func (io InfluxOutput) Battery() {
	io.print("Battery", GetBattery())
}

func (io InfluxOutput) Current() {
	io.print("Current", GetCurrent())
}

func (io InfluxOutput) Fans() {
	io.print("Fans", GetFans())
}

func (io InfluxOutput) Hardware() {
	io.print("Hardware", GetHardware())
}

func (io InfluxOutput) Power() {
	io.print("Power", GetPower())
}

func (io InfluxOutput) Temperature() {
	io.print("Temperature", GetTemperature())
}

func (io InfluxOutput) Voltage() {
	io.print("Voltage", GetVoltage())
}

// influxStringConvert returns s converted to lowercase with spaces replaced by underscores,
// suitable for use as an InfluxDB measurement or tag value.
func influxStringConvert(s string) string {
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.ToLower(s)

	return s
}

// influxGetValue returns the numeric part of a "value unit" formatted sensor string.
func influxGetValue(s string) string {
	val, _, _ := strings.Cut(s, " ")

	return val
}

// influxGetUnit returns the unit portion of a "value=<number> <unit>" sensor string,
// stripped of any degree symbol and lowercased. Returns "none" when no unit is present.
func influxGetUnit(s string) string {
	s = strings.TrimPrefix(s, "value=")

	_, unit, found := strings.Cut(s, " ")
	if !found {
		return "none"
	}

	unit = strings.ReplaceAll(unit, "°", "")

	return strings.ToLower(unit)
}

// print writes smcdata to stdout in InfluxDB line protocol format, tagged with the sensor type name.
// It is a no-op when smcdata is empty.
func (io InfluxOutput) print(name string, smcdata map[string]any) {
	if len(smcdata) != 0 {
		ct := time.Now().UnixNano()

		for _, k := range sortedKeys(smcdata) {
			v := smcdata[k]
			if sensorMap, ok := v.(map[string]any); ok {
				var key string
				if keyStr, ok := sensorMap["key"].(string); ok && keyStr != "" {
					key = influxStringConvert(fmt.Sprintf(",key=%s", keyStr))
				}

				value := fmt.Sprintf("value=%v", sensorMap["value"])
				unit := influxGetUnit(value)

				fmt.Fprintf(io.writer, "%v,sensortype=%s,unit=%s%s %s %d\n",
					influxStringConvert(k),
					influxStringConvert(name),
					unit,
					key,
					influxGetValue(value),
					ct)
			}
		}
	}
}
