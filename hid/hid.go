// Copyright (C) 2022  Dinko Korunic
// SPDX-License-Identifier: GPL-3.0-only

//go:build darwin

package hid

import "C"

import (
	"bufio"
	"fmt"
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

		// Match smc.isValidReading near-zero threshold.
		if val < 0.005 {
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
