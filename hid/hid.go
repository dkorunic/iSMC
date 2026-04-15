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

import "C"

import (
	"bufio"
	"fmt"
	"math"
	"strconv"
	"strings"
)

const (
	SensorSeparator = ":"
	SensorType      = "hid"
)

// getGeneric returns a map of HID sensor stats.
func getGeneric(unit string, cStr *C.char) map[string]any {
	goStr := C.GoString(cStr)
	generic := make(map[string]any)

	scanner := bufio.NewScanner(strings.NewReader(goStr))
	for scanner.Scan() {
		split := strings.SplitN(scanner.Text(), SensorSeparator, 2)
		if len(split) != 2 {
			continue
		}

		val, err := strconv.ParseFloat(split[1], 32)
		if err != nil {
			continue
		}

		if val <= 0.0 || math.Round(val*100)/100 == 0.0 {
			continue
		}

		name := split[0]
		key := ""

		if i := strings.LastIndexByte(name, ' '); i >= 0 {
			key = name[i+1:]
		}

		generic[name] = map[string]any{
			"key":   key,
			"value": fmt.Sprintf("%g %s", float32(val), unit),
			"type":  SensorType,
		}
	}

	return generic
}
