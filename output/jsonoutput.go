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
	"strconv"
	"strings"

	"github.com/goccy/go-json"
)

type JSONOutput struct {
	writer io.Writer
}

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

func format(d any) (any, error) {
	v, ok := d.(map[string]any)
	if !ok {
		return v, fmt.Errorf("not a map")
	}

	for key, entry := range v {
		t, err := json.Marshal(entry)
		if err != nil {
			return v, err
		}

		buf := newstruct{}
		err = json.Unmarshal(t, &buf)
		if err != nil {
			return v, err
		}

		// Process only string values
		switch buf.Value.(type) {
		case string:
			if !strings.Contains(buf.Value.(string), " ") {
				continue
			}
			s := strings.Split(buf.Value.(string), " ")
			switch buf.Type {
			case "flt", "hid", "fp1f", "fp2e", "fp3d", "fp4c", "fp5b", "fp6a", "fp79", "fp88", "fpa6", "fpc4", "fpe2", "sp1e", "sp2d", "sp3c", "sp4b", "sp5a", "sp69", "sp78", "sp87", "sp96", "spa5", "spb4", "spf0":
				f, err := strconv.ParseFloat(s[0], 64)
				if err != nil {
					return v, err
				}

				buf.Quantity = f
				buf.Unit = s[1]
			}
		}

		v[key] = buf
	}

	return v, nil
}

func (jo JSONOutput) All() {
	var err error
	data := GetAll()
	for key, d := range data {
		if data[key], err = format(d); err != nil {
			jo.print(GetAll())
			return
		}
	}

	out, _ := json.Marshal(data)
	fmt.Println(string(out))
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

func (jo JSONOutput) print(v any) {
	data, err := format(v)
	if err != nil {
		fmt.Printf("could not format data: %v\n", err)
		return
	}

	out, _ := json.Marshal(data)
	fmt.Println(string(out))
}
