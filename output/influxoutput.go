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
	"sort"
	"strings"
	"time"

	"github.com/fvbommel/sortorder"
)

type InfluxOutput struct {
	writer io.Writer
}

func NewInfluxOutput() Output {
	o := InfluxOutput{}
	o.writer = io.Writer(os.Stdout)

	return o
}

func (io InfluxOutput) All() {
	all := GetAll()

	keys := make([]string, 0, len(all))
	for k := range all {
		keys = append(keys, k)
	}
	sort.Sort(sortorder.Natural(keys))

	for _, key := range keys {
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

func (io InfluxOutput) Power() {
	io.print("Power", GetPower())
}

func (io InfluxOutput) Temperature() {
	io.print("Temperature", GetTemperature())
}

func (io InfluxOutput) Voltage() {
	io.print("Voltage", GetVoltage())
}

func influx_string_convert(s string) string {
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.ToLower(s)
	return s
}

func influx_get_value(s string) string {
	s = strings.Split(s, " ")[0]
	return s
}

func influx_get_unit(s string) string {
	s = strings.Trim(s, "value=")
	if len(strings.Split(s, " ")) > 1 {
		s = strings.Split(s, " ")[1]
		s = strings.ReplaceAll(s, "°", "")
		s = strings.ToLower(s)
	} else {
		s = "none"
	}
	return s
}

func (io InfluxOutput) print(name string, smcdata map[string]any) {
	if len(smcdata) != 0 {
		ct := time.Now().UnixNano()

		keys := make([]string, 0, len(smcdata))
		for k := range smcdata {
			keys = append(keys, k)
		}

		for _, k := range keys {
			v := smcdata[k]
			if value, ok := v.(map[string]any); ok {
				key := fmt.Sprint(value["key"])
				if len(key) > 0 {
					key = influx_string_convert(fmt.Sprintf(",key=%s", value["key"]))
				} else {
					key = ""
				}
				value := fmt.Sprintf("value=%v", value["value"])
				unit := influx_get_unit(fmt.Sprintf("%v", value))
				fmt.Printf("%v,sensortype=%s,unit=%s%s %s %d\n",
					influx_string_convert(k),
					influx_string_convert(name),
					unit,
					key,
					influx_get_value(value),
					ct)
			}
		}
	}
}
