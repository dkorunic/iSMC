// SPDX-FileCopyrightText: Copyright (C) 2022 Roland Schaer
// SPDX-License-Identifier: GPL-3.0-only

//go:build darwin

package output

import (
	"fmt"
	"io"
	"os"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
)

type TableOutput struct {
	writer  io.Writer
	isASCII bool
}

// NewTableOutput returns a TableOutput that writes to stdout. When isASCII is true
// the output uses plain ASCII borders; otherwise a coloured style is applied.
func NewTableOutput(isASCII bool) Output {
	return TableOutput{
		isASCII: isASCII,
		writer:  os.Stdout,
	}
}

func (to TableOutput) All() {
	all := GetAll()

	for _, key := range sortedKeys(all) {
		value := all[key]
		if smcdata, ok := value.(map[string]any); ok {
			to.print(key, smcdata)
		}
	}
}

func (to TableOutput) Battery() {
	to.print("Battery", GetBattery())
}

func (to TableOutput) Current() {
	to.print("Current", GetCurrent())
}

func (to TableOutput) Fans() {
	to.print("Fans", GetFans())
}

func (to TableOutput) Hardware() {
	to.print("Hardware", GetHardware())
}

func (to TableOutput) Power() {
	to.print("Power", GetPower())
}

func (to TableOutput) Temperature() {
	to.print("Temperature", GetTemperature())
}

func (to TableOutput) Voltage() {
	to.print("Voltage", GetVoltage())
}

// print renders smcdata as a formatted table with the given title, sorted by natural key order.
// It is a no-op when smcdata is empty.
func (to TableOutput) print(name string, smcdata map[string]any) {
	if len(smcdata) == 0 {
		return
	}

	t := table.NewWriter()
	t.SetOutputMirror(to.writer)

	if !to.isASCII {
		t.SetStyle(table.StyleColoredBright)
	}

	t.SetTitle(name)

	t.Style().Title.Align = text.AlignCenter
	t.AppendHeader(table.Row{"Description", "Key", "Value", "Type"})

	for _, k := range sortedKeys(smcdata) {
		v := smcdata[k]
		if value, ok := v.(map[string]any); ok {
			t.AppendRow([]any{
				k,
				value["key"],
				fmt.Sprintf("%8v", value["value"]),
				value["type"],
			})
		}
	}

	t.Render()
	fmt.Fprintln(to.writer)
}
