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
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/fvbommel/sortorder"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
)

type TableOutput struct {
	isASCII bool
	writer  io.Writer
}

func NewTableOutput(isASCII bool) Output {
	o := TableOutput{}
	o.isASCII = isASCII
	o.writer = io.Writer(os.Stdout)

	return o
}

func (to TableOutput) All() {
	all := GetAll()

	keys := make([]string, 0, len(all))
	for k := range all {
		keys = append(keys, k)
	}
	sort.Sort(sortorder.Natural(keys))

	for _, key := range keys {
		value := all[key]
		smcdata := value.(map[string]interface{})
		to.print(key, smcdata)
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

func (to TableOutput) Power() {
	to.print("Power", GetPower())
}

func (to TableOutput) Temperature() {
	to.print("Temperature", GetTemperature())
}

func (to TableOutput) Voltage() {
	to.print("Voltage", GetVoltage())
}

func (to TableOutput) print(name string, smcdata map[string]interface{}) {
	if len(smcdata) != 0 {
		t := table.NewWriter()
		t.SetOutputMirror(to.writer)
		if !to.isASCII {
			t.SetStyle(table.StyleColoredBright)
		}
		t.SetTitle(name)
		t.Style().Title.Align = text.AlignCenter
		t.AppendHeader(table.Row{"Description", "Key", "Value", "Type"})

		keys := make([]string, 0, len(smcdata))
		for k := range smcdata {
			keys = append(keys, k)
		}
		sort.Sort(sortorder.Natural(keys))

		for _, k := range keys {
			v := smcdata[k]
			value := v.(map[string]interface{})
			t.AppendRow([]interface{}{
				fmt.Sprintf("%v", k),
				value["key"],
				fmt.Sprintf("%8v", value["value"]),
				value["type"],
			})
		}

		t.Render()
		fmt.Fprintln(to.writer)
	}
}
