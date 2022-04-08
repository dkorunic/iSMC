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

	jsoniter "github.com/json-iterator/go"
)

type JSONOutput struct {
	writer io.Writer
}

func NewJSONOutput() Output {
	o := JSONOutput{}
	o.writer = io.Writer(os.Stdout)

	return o
}

func (jo JSONOutput) All() {
	jo.print(GetAll())
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

func (jo JSONOutput) Power() {
	jo.print(GetPower())
}

func (jo JSONOutput) Temperature() {
	jo.print(GetTemperature())
}

func (jo JSONOutput) Voltage() {
	jo.print(GetVoltage())
}

func (jo JSONOutput) print(v interface{}) {
	json := jsoniter.ConfigCompatibleWithStandardLibrary
	out, _ := json.MarshalIndent(v, "", "  ")
	fmt.Fprint(jo.writer, string(out))
}
