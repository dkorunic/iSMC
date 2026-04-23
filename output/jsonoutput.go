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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

type JSONOutput struct {
	writer io.Writer
}

// NewJSONOutput returns a JSONOutput that writes to stdout.
func NewJSONOutput() Output {
	o := JSONOutput{}
	o.writer = io.Writer(os.Stdout)

	return o
}

type newstruct struct {
	Key      string `json:"key"`
	Type     string `json:"type"`
	Value    any    `json:"value"`
	Quantity any    `json:"quantity"`
	Unit     string `json:"unit"`
}

// format enriches sensor map entries that carry a space-separated "value unit" string by
// parsing the numeric part into a Quantity field and the unit into a Unit field.
// It returns the enriched value, or an error only if d is not a map. Entries whose
// numeric part fails to parse are skipped (the original entry is left intact) rather
// than aborting the whole output, so one malformed sensor cannot suppress all others.
// Note: format mutates the input map in place; callers must not pass shared or cached maps.
func format(d any) (any, error) {
	v, ok := d.(map[string]any)
	if !ok {
		return v, errors.New("not a map")
	}

	for key, entry := range v {
		sensorMap, ok := entry.(map[string]any)
		if !ok {
			continue
		}

		// Require "number unit" string.
		valStr, ok := sensorMap["value"].(string)
		if !ok || !strings.Contains(valStr, " ") {
			continue
		}

		smcKey, _ := sensorMap["key"].(string)
		typ, _ := sensorMap["type"].(string)

		buf := newstruct{
			Key:   smcKey,
			Type:  typ,
			Value: valStr,
		}

		if isFloatType(typ) {
			numStr, unit, _ := strings.Cut(valStr, " ")

			f, err := strconv.ParseFloat(numStr, 64)
			if err != nil {
				// Skip unparseable entry; do not fail whole output.
				continue
			}

			buf.Quantity = f
			buf.Unit = unit
		}

		v[key] = buf
	}

	return v, nil
}

func (jo JSONOutput) All() {
	data := GetAll()

	for key, d := range data {
		enriched, err := format(d)
		if err != nil {
			fmt.Fprintf(os.Stderr, "could not format data: %v\n", err)

			return
		}

		data[key] = enriched
	}

	out, err := json.Marshal(data)
	if err != nil {
		fmt.Fprintf(jo.writer, "could not marshal data: %v\n", err)

		return
	}

	fmt.Fprintln(jo.writer, string(out))
}

func (jo JSONOutput) Battery() {
	jo.print(GetBattery())
}

func (jo JSONOutput) Current() {
	jo.print(GetCurrent())
}

func (jo JSONOutput) Fans() {
	jo.print(GetFans())
}

func (jo JSONOutput) Hardware() {
	jo.print(GetHardware())
}

func (jo JSONOutput) Power() {
	jo.print(GetPower())
}

func (jo JSONOutput) Temperature() {
	jo.print(GetTemperature())
}

func (jo JSONOutput) Voltage() {
	jo.print(GetVoltage())
}

// print formats v via format and writes the resulting JSON to the writer.
// Formatting and marshaling errors are reported to stderr; the writer receives no output on error.
func (jo JSONOutput) print(v any) {
	data, err := format(v)
	if err != nil {
		fmt.Fprintf(os.Stderr, "could not format data: %v\n", err)

		return
	}

	out, err := json.Marshal(data)
	if err != nil {
		fmt.Fprintf(os.Stderr, "could not marshal data: %v\n", err)

		return
	}

	fmt.Fprintln(jo.writer, string(out))
}
