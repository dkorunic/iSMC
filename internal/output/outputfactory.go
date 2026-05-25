// SPDX-FileCopyrightText: Copyright (C) 2022 Roland Schaer
// SPDX-License-Identifier: GPL-3.0-only

//go:build darwin

package output

// Factory returns the Output implementation for the given format name.
// Recognised values are "table", "json", and "influx"; anything else falls back to ASCII table output.
func Factory(outputType string) Output {
	switch outputType {
	case "table":
		return NewTableOutput(false)
	case "ascii":
		return NewTableOutput(true)
	case "json":
		return NewJSONOutput()
	case "influx":
		return NewInfluxOutput()
	default:
		return NewTableOutput(true)
	}
}
