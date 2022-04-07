// Copyright (C) 2022  Dinko Korunic
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

package hid

import (
	"C"
	"bufio"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/jedib0t/go-pretty/table"
)

const (
	SensorSeparator = ":"
	SensorType      = "hid"
)

// SensorStat holds HID sensors names and values.
type SensorStat struct {
	Name  string  // HID sensor name
	Value float32 // HID sensor readout
}

// printGeneric prints a table of HID sensors (description) and values with units.
func printGeneric(t table.Writer, unit string, cStr *C.char) {
	var stats []SensorStat
	goStr := C.GoString(cStr)
	scanner := bufio.NewScanner(strings.NewReader(goStr))
	for scanner.Scan() {
		split := strings.Split(scanner.Text(), SensorSeparator)
		if len(split) != 2 {
			continue
		}

		val, err := strconv.ParseFloat(split[1], 32)
		if err != nil {
			continue
		}

		if val != -127.0 && val != 0.0 {
			if val < 0.0 {
				val = -val
			}

			stats = append(stats, SensorStat{
				Name:  split[0],
				Value: float32(val),
			})
		}
	}

	sort.Slice(stats, func(i, j int) bool {
		return stats[i].Name < stats[j].Name
	})

	for _, v := range stats {
		name := v.Name
		val := v.Value
		t.AppendRow([]interface{}{
			name,
			"",
			fmt.Sprintf("%6.1f %s", val, unit),
			SensorType,
		})
	}
}
