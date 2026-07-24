// SPDX-FileCopyrightText: Copyright (C) 2023 Seaburr
// SPDX-License-Identifier: GPL-3.0-only

//go:build darwin

package output

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

// influxNoUnit is the unit tag emitted for dimensionless or free-text fields.
const influxNoUnit = "none"

type InfluxOutput struct {
	writer io.Writer
}

// NewInfluxOutput returns an InfluxOutput that writes to stdout.
func NewInfluxOutput() Output {
	return InfluxOutput{writer: os.Stdout}
}

func (o InfluxOutput) All() {
	all := GetAll()

	for _, key := range sortedKeys(all) {
		value := all[key]
		if smcdata, ok := value.(map[string]any); ok {
			o.print(key, smcdata)
		}
	}
}

func (o InfluxOutput) Battery() {
	o.print("Battery", GetBattery())
}

func (o InfluxOutput) Current() {
	o.print("Current", GetCurrent())
}

func (o InfluxOutput) Fans() {
	o.print("Fans", GetFans())
}

func (o InfluxOutput) Hardware() {
	o.print("Hardware", GetHardware())
}

func (o InfluxOutput) Power() {
	o.print("Power", GetPower())
}

func (o InfluxOutput) Temperature() {
	o.print("Temperature", GetTemperature())
}

func (o InfluxOutput) Voltage() {
	o.print("Voltage", GetVoltage())
}

// influxStringConvert lowercases s and replaces spaces with underscores for use as
// a measurement or tag value.
func influxStringConvert(s string) string {
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.ToLower(s)

	return s
}

// influxEscape backslash-escapes line-protocol specials (comma, equals, space) and
// drops record-terminating newlines, guarding against delimiters in sensor text.
func influxEscape(s string) string {
	if !strings.ContainsAny(s, ",= \n\r") {
		return s
	}

	var b strings.Builder

	b.Grow(len(s) + 4)

	for i := range len(s) {
		c := s[i]
		if c == '\n' || c == '\r' {
			continue
		}

		if c == ',' || c == '=' || c == ' ' {
			b.WriteByte('\\')
		}

		b.WriteByte(c)
	}

	return b.String()
}

// influxFieldValue maps a raw sensor value to a line-protocol field and unit tag.
// Free text is double-quoted because an unquoted space or comma (e.g. "Mac13,2")
// corrupts the line; "<number> <unit>" strings split into a bare value and unit.
func influxFieldValue(v any) (string, string) {
	switch val := v.(type) {
	case bool:
		return strconv.FormatBool(val), influxNoUnit
	case string:
		return influxStringFieldValue(val)
	default:
		return fmt.Sprintf("%v", val), influxNoUnit
	}
}

// influxStringFieldValue splits "<number> <unit>" into value and unit, else quotes
// the whole string. strings.Fields also absorbs the leading space that "%4.0f rpm"
// padding produces on sub-1000 fan speeds.
func influxStringFieldValue(s string) (string, string) {
	fields := strings.Fields(s)

	if len(fields) > 0 {
		_, err := strconv.ParseFloat(fields[0], 64)
		if err == nil {
			unit := influxNoUnit
			if len(fields) > 1 {
				unit = strings.ToLower(strings.ReplaceAll(strings.Join(fields[1:], " "), "°", ""))
			}

			return fields[0], unit
		}
	}

	return influxQuoteField(s), influxNoUnit
}

// influxQuoteField double-quotes s as a string field, escaping embedded quotes and
// backslashes and dropping record-terminating newlines.
func influxQuoteField(s string) string {
	var b strings.Builder

	b.Grow(len(s) + 2)
	b.WriteByte('"')

	for i := range len(s) {
		c := s[i]
		if c == '\n' || c == '\r' {
			continue
		}

		if c == '"' || c == '\\' {
			b.WriteByte('\\')
		}

		b.WriteByte(c)
	}

	b.WriteByte('"')

	return b.String()
}

// print emits smcdata as InfluxDB line protocol, tagged with the sensor type name.
func (o InfluxOutput) print(name string, smcdata map[string]any) {
	if len(smcdata) == 0 {
		return
	}

	ct := time.Now().UnixNano()

	for _, k := range sortedKeys(smcdata) {
		v := smcdata[k]

		sensorMap, ok := v.(map[string]any)
		if !ok {
			continue
		}

		// Escape only the user value, not the "=" separator.
		var key string
		if keyStr, ok := sensorMap["key"].(string); ok && keyStr != "" {
			key = ",key=" + influxEscape(influxStringConvert(keyStr))
		}

		field, unit := influxFieldValue(sensorMap["value"])

		fmt.Fprintf(o.writer, "%v,sensortype=%s,unit=%s%s value=%s %d\n",
			influxEscape(influxStringConvert(k)),
			influxEscape(influxStringConvert(name)),
			influxEscape(unit),
			key,
			field,
			ct)
	}
}
