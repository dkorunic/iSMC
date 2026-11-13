// SPDX-FileCopyrightText: Copyright (C) 2022 Roland Schaer
// SPDX-License-Identifier: GPL-3.0-only

//go:build darwin

package output

import "time"

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

// Run prints one category set (show is a method expression such as
// Output.Temperature) in format. With every > 0 it keeps going: the
// interactive table view refreshes in place until the user quits, and every
// other output re-prints each interval until interrupted.
func Run(format string, every time.Duration, show func(Output)) {
	o := Factory(format)

	if to, ok := o.(TableOutput); ok && every > 0 && to.interactive() {
		to.every = every
		show(to)

		return
	}

	show(o)

	if every <= 0 {
		return
	}

	for range time.Tick(every) {
		show(o)
	}
}
