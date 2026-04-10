// Copyright (C) 2019  Dinko Korunic
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

package smc

import (
	"fmt"
	"math"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/dkorunic/iSMC/gosmc"
	"github.com/dkorunic/iSMC/platform"
)

const (
	AppleSMC    = "AppleSMC"
	FanNum      = "FNum"
	BattNum     = "BNum"
	BattPwr     = "BATP"
	BattInf     = "BSIn"
	KeyWildcard = "%"
)

// SensorStat is SMC key to description mapping.
type SensorStat struct {
	Key      string
	Desc     string
	Platform string
}

//go:generate ./gen-sensors.sh sensors.go

// GetAll returns all SMC sensor readings grouped by category (Battery, Current, Fans, Temperature, Power, Voltage).
func GetAll() map[string]any {
	sensors := make(map[string]any)

	sensors["Battery"] = GetBattery()
	sensors["Current"] = GetCurrent()
	sensors["Fans"] = GetFans()
	sensors["Temperature"] = GetTemperature()
	sensors["Power"] = GetPower()
	sensors["Voltage"] = GetVoltage()

	return sensors
}

// GetBattery returns battery count, status flags, and AC power state read from SMC keys.
func GetBattery() map[string]any {
	c, res := gosmc.SMCOpen(AppleSMC)
	if res != gosmc.IOReturnSuccess {
		fmt.Fprintf(os.Stderr, "Unable to open Apple SMC; return code %v\n", res)
		os.Exit(1)
	}
	defer gosmc.SMCClose(c)

	n, ty1, _ := getKeyUint32(c, BattNum) // Get number of batteries
	i, ty2, _ := getKeyUint32(c, BattInf) // Get battery info (needs bit decoding)
	b, ty3, _ := getKeyBool(c, BattPwr)   // Get AC status

	battery := map[string]any{
		"Battery Count": map[string]any{
			"key":   BattNum,
			"value": n,
			"type":  ty1,
		},
		"Battery Info": map[string]any{
			"key":   BattInf,
			"value": i,
			"type":  ty2,
		},
		"Battery Power": map[string]any{
			"key":   BattPwr,
			"value": b,
			"type":  ty3,
		},
	}

	return battery
}

// GetCurrent returns current sensor readings (in amperes) from SMC.
func GetCurrent() map[string]any {
	return getGeneric("Current", "A", AppleCurrent)
}

// GetFans returns fan count and per-fan speed readings (in RPM) from SMC.
func GetFans() map[string]any {
	c, res := gosmc.SMCOpen(AppleSMC)
	if res != gosmc.IOReturnSuccess {
		fmt.Fprintf(os.Stderr, "Unable to open Apple SMC; return code %v\n", res)
		os.Exit(1)
	}
	defer gosmc.SMCClose(c)

	fans := make(map[string]any)

	val, smcType, _ := getKeyUint32(c, FanNum) // Get number of fans
	fans["Fan Count"] = map[string]any{
		"key":   FanNum,
		"value": val,
		"type":  smcType,
	}

	for i := range val {
		for _, v := range AppleFans {
			key := fmt.Sprintf(v.Key, i)
			desc := fmt.Sprintf(v.Desc, i+1)

			val, smcType, err := getKeyFloat32(c, key)
			if err != nil {
				continue
			}

			if val > 0.0 && math.Round(float64(val)*100)/100 != 0.0 {
				fans[desc] = map[string]any{
					"key":   key,
					"value": fmt.Sprintf("%4.0f rpm", val),
					"type":  smcType,
				}
			}
		}
	}

	return fans
}

// getGeneric reads each sensor in smcSlice from SMC, expanding wildcard keys (%) to indices 0–9,
// and returns a map of description → sensor entry formatted with the given unit string.
func getGeneric(_, unit string, smcSlice []SensorStat) map[string]any {
	conn, res := gosmc.SMCOpen(AppleSMC)
	if res != gosmc.IOReturnSuccess {
		fmt.Fprintf(os.Stderr, "Unable to open Apple SMC; return code %v\n", res)
		os.Exit(1)
	}
	defer gosmc.SMCClose(conn)

	generic := make(map[string]any)

	for _, v := range smcSlice {
		key := v.Key
		desc := v.Desc

		if !strings.Contains(key, KeyWildcard) {
			addGeneric(generic, conn, key, desc, unit)

			continue
		}

		for i := range 10 {
			iKey := strings.Replace(key, KeyWildcard, strconv.Itoa(i), 1)
			iDesc := strings.Replace(desc, KeyWildcard, strconv.Itoa(i+1), 1)
			addGeneric(generic, conn, iKey, iDesc, unit)
		}
	}

	return generic
}

// addGeneric reads a single SMC key and adds the result to generic under desc if the value is
// valid (non-zero, non-negative sentinel, and non-negligible after rounding).
func addGeneric(generic map[string]any, conn uint, key, desc, unit string) {
	val, smcType, err := getKeyFloat32(conn, key)
	if err != nil {
		return
	}

	if val > 0.0 && math.Round(float64(val)*100)/100 != 0.0 {
		generic[desc] = map[string]any{
			"key":   key,
			"value": fmt.Sprintf("%g %s", val, unit),
			"type":  smcType,
		}
	}
}

// GetPower returns power sensor readings (in watts) from SMC.
func GetPower() map[string]any {
	return getGeneric("Power", "W", ApplePower)
}

// GetTemperature returns temperature sensor readings (in °C) from SMC, filtered to the detected platform family.
func GetTemperature() map[string]any {
	return getGeneric("Temperature", "°C", filterForPlatform(AppleTemp))
}

// GetVoltage returns voltage sensor readings (in volts) from SMC.
func GetVoltage() map[string]any {
	return getGeneric("Voltage", "V", AppleVoltage)
}

// filterForPlatform returns the subset of smcSlice whose Platform tag matches the detected hardware
// family (e.g. "M1", "Intel"). Sensors tagged "All" or "" are always included; sensors tagged
// "Apple" are included for any Apple Silicon family. Falls back to runtime architecture when the
// model cannot be identified.
func filterForPlatform(smcSlice []SensorStat) []SensorStat {
	filteredSensors := make([]SensorStat, 0, len(smcSlice))

	family := platform.GetFamily()
	if family == "" || family == "Unknown" {
		switch runtime.GOARCH {
		case "arm64":
			family = "Apple"
		case "amd64", "386":
			family = "Intel"
		}
	}

	familyApple := strings.HasPrefix(family, "M") || strings.HasPrefix(family, "A") || family == "Apple"

	for _, v := range smcSlice {
		// Generic/common sensors in Apple Silicon family
		if v.Platform == "Apple" && familyApple {
			filteredSensors = append(filteredSensors, v)

			continue
		}

		// Generic/common sensors
		if v.Platform == "" || v.Platform == "All" {
			filteredSensors = append(filteredSensors, v)

			continue
		}

		// Platform-specific sensors for Intel or M-family
		if v.Platform == family {
			filteredSensors = append(filteredSensors, v)

			continue
		}
	}

	return filteredSensors
}
