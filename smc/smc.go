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
	"strconv"
	"strings"

	"github.com/panotza/gosmc"
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
	// SMC key name
	Key string
	// SMC key description
	Desc string
}

//go:generate ./gen-sensors.sh sensors.go

func GetAll() map[string]any { // Get all sensors
	sensors := make(map[string]any)

	sensors["Battery"] = GetBattery()
	sensors["Current"] = GetCurrent()
	sensors["Fans"] = GetFans()
	sensors["Temperature"] = GetTemperature()
	sensors["Power"] = GetPower()
	sensors["Voltage"] = GetVoltage()

	return sensors
}

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

func GetCurrent() map[string]any {
	return getGeneric("Current", "A", AppleCurrent)
}

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

			if val != -127.0 && val != 0.0 && math.Round(float64(val)*100)/100 != 0.0 {
				if val < 0.0 {
					val = -val
				}

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

func addGeneric(generic map[string]any, conn uint, key, desc, unit string) {
	val, smcType, err := getKeyFloat32(conn, key)
	if err != nil {
		return
	}

	if val != -127.0 && val != 0.0 && math.Round(float64(val)*100)/100 != 0.0 {
		if val < 0.0 {
			val = -val
		}

		generic[desc] = map[string]any{
			"key":   key,
			"value": fmt.Sprintf("%.1f %s", val, unit),
			"type":  smcType,
		}
	}
}

func GetPower() map[string]any {
	return getGeneric("Power", "W", ApplePower)
}

func GetTemperature() map[string]any {
	return getGeneric("Temperature", "°C", AppleTemp)
}

func GetVoltage() map[string]any {
	return getGeneric("Voltage", "V", AppleVoltage)
}
