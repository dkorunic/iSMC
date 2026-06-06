// SPDX-FileCopyrightText: Copyright (C) 2022 Roland Schaer
// SPDX-License-Identifier: GPL-3.0-only

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
	return JSONOutput{writer: os.Stdout}
}

type sensorEntry struct {
	Key      string `json:"key"`
	Type     string `json:"type"`
	Value    any    `json:"value"`
	Quantity any    `json:"quantity"`
	Unit     string `json:"unit"`
}

// format parses "value unit" entries into separate Quantity/Unit fields.
// Errors only if d is not a map. Unparseable entries are skipped, not fatal.
// Mutates the input map; callers must not pass shared or cached maps.
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

		valStr, ok := sensorMap["value"].(string)
		if !ok || !strings.Contains(valStr, " ") {
			continue
		}

		smcKey, _ := sensorMap["key"].(string)
		typ, _ := sensorMap["type"].(string)

		buf := sensorEntry{
			Key:   smcKey,
			Type:  typ,
			Value: valStr,
		}

		if isFloatType(typ) {
			numStr, unit, _ := strings.Cut(valStr, " ")

			f, err := strconv.ParseFloat(numStr, 64)
			if err != nil {
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
