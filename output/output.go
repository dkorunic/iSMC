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
	jsoniter "github.com/json-iterator/go"

	"github.com/dkorunic/iSMC/hid"
	"github.com/dkorunic/iSMC/smc"
)

// monkey patching for testing
var (
	GetAll         = getAll
	GetTemperature = getTemperature
	GetFans        = getFans
	GetBattery     = getBattery
	GetPower       = getPower
	GetVoltage     = getVoltage
	GetCurrent     = getCurrent
)

type Output interface {
	// All prints all the detected sensors results
	All()
	// Battery prints the detected battery sensor results
	Battery()
	// Current prints the current sensor results
	Current()
	// Fans prints the detected fan sensor results
	Fans()
	// Temperature prints detected temperature sensor results
	Temperature()
	// Power prints detected power sensor results
	Power()
	// Voltage prints detected voltage sensor results
	Voltage()
}

func getAll() map[string]interface{} {
	return merge(smc.GetAll(), hid.GetAll())
}

func getBattery() map[string]interface{} {
	return smc.GetBattery()
}

func getCurrent() map[string]interface{} {
	merged := make(map[string]interface{})
	deepCopy(merged, smc.GetCurrent())
	deepCopy(merged, hid.GetCurrent())

	return merged
}

func getFans() map[string]interface{} {
	return smc.GetFans()
}

func getTemperature() map[string]interface{} {
	merged := make(map[string]interface{})
	deepCopy(merged, smc.GetTemperature())
	deepCopy(merged, hid.GetTemperature())

	return merged
}

func getPower() map[string]interface{} {
	return smc.GetPower()
}

func getVoltage() map[string]interface{} {
	merged := make(map[string]interface{})
	deepCopy(merged, smc.GetVoltage())
	deepCopy(merged, hid.GetVoltage())

	return merged
}

// TODO replace with a variant from an utility package
func deepCopy(dest, src map[string]interface{}) {
	json := jsoniter.ConfigCompatibleWithStandardLibrary
	jsonStr, _ := json.Marshal(src)
	_ = json.Unmarshal(jsonStr, &dest)
}

// TODO replace with a variant from an utility package
func merge(a, b map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{})
	deepCopy(out, a)

	for k, v := range b {
		if v, ok := v.(map[string]interface{}); ok {
			if bv, ok := out[k]; ok {
				if bv, ok := bv.(map[string]interface{}); ok {
					out[k] = merge(bv, v)

					continue
				}
			}
		}
		out[k] = v
	}

	return out
}
