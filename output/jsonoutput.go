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
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

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

type newstruct struct {
	Key      string `json:"key"`
	Type     string `json:"type"`
	Value    any    `json:"value"`
	Quantity any    `json:"quantity"`
	Unit     string `json:"unit"`
}

func format(d any) (any, error) {
	v := d.(map[string]any)
	var err error
	for key, entry := range v {
		t := make([]byte, 0)
		buf := new(newstruct)
		s := make([]string, 0)
		t, err = json.Marshal(entry)
		if err != nil {
			fmt.Println("json.Marshal error:", err)
			break
		}
		err = json.Unmarshal(t, buf)
		if err != nil {
			fmt.Println("json.Unmarshal error:", err)
			break
		}
		// Process only string values
		switch buf.Value.(type) {
		case string:
			if !strings.Contains(buf.Value.(string), " ") {
				continue
			}
			s = strings.Split(buf.Value.(string), " ")
			switch buf.Type {
			case "flt", "sp78", "sp87":
				f, err := strconv.ParseFloat(s[0], 64)
				if err != nil {
					break
				}
				buf.Quantity = f
				buf.Unit = s[1]
			}
		}
		v[key] = buf
	}
	return v, err

}

func (jo JSONOutput) All() {
	var err error
	data := make(map[string]any)
	data = GetAll()
	for key, d := range data {
		if data[key], err = format(d); err != nil {
			fmt.Printf("Convert error:%v\n", err)
			jo.print(GetAll())
			return
		}
	}
	json := jsoniter.ConfigCompatibleWithStandardLibrary
	out, _ := json.MarshalIndent(data, "", "  ")
	fmt.Fprint(jo.writer, string(out))
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
	data, err := convert(v)
	if err != nil {
		fmt.Println("Convert error:", v)
		return
	}
	json := jsoniter.ConfigCompatibleWithStandardLibrary
	out, _ := json.MarshalIndent(data, "", "  ")
	fmt.Fprint(jo.writer, string(out))
}
