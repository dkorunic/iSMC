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
	"sort"
	"strconv"

	"github.com/dkorunic/iSMC/hid"
	"github.com/dkorunic/iSMC/platform"
	"github.com/dkorunic/iSMC/smc"
	"github.com/fvbommel/sortorder"
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
	GetHardware    = getHardware
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
	// Hardware prints the detected hardware information
	Hardware()
	// Temperature prints detected temperature sensor results
	Temperature()
	// Power prints detected power sensor results
	Power()
	// Voltage prints detected voltage sensor results
	Voltage()
}

// getAll returns all sensor data by merging SMC and HID results.
func getAll() map[string]any {
	return merge(smc.GetAll(), hid.GetAll())
}

// getBattery returns battery sensor data from SMC.
func getBattery() map[string]any {
	return smc.GetBattery()
}

// getCurrent returns current sensor data merged from SMC and HID sources.
func getCurrent() map[string]any {
	return merge(smc.GetCurrent(), hid.GetCurrent())
}

// getFans returns fan sensor data from SMC.
func getFans() map[string]any {
	return smc.GetFans()
}

// getTemperature returns temperature sensor data merged from SMC and HID sources.
func getTemperature() map[string]any {
	return merge(smc.GetTemperature(), hid.GetTemperature())
}

// getPower returns power sensor data from SMC.
func getPower() map[string]any {
	return smc.GetPower()
}

// getVoltage returns voltage sensor data merged from SMC and HID sources.
func getVoltage() map[string]any {
	return merge(smc.GetVoltage(), hid.GetVoltage())
}

// sortedKeys returns the keys of m sorted in natural order.
func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}

	sort.Sort(sortorder.Natural(keys))

	return keys
}

// getHardware returns hardware information gathered from platform detection and sysctls,
// including model name, CPU family, CPU model, year, and per-cluster core counts.
func getHardware() map[string]any {
	result := make(map[string]any)

	modelID := platform.GetModelID()
	if modelID != "" {
		result["Model Identifier"] = map[string]any{
			"key":   "hw.model",
			"value": modelID,
			"type":  "sysctl",
		}
	}

	product, ok := platform.GetProduct()
	if ok {
		result["Mac Model"] = map[string]any{
			"key":   "hw.model",
			"value": product.Name,
			"type":  "platform",
		}
		result["Platform Family"] = map[string]any{
			"key":   "hw.family",
			"value": product.Family,
			"type":  "platform",
		}
		result["CPU"] = map[string]any{
			"key":   "hw.cpu",
			"value": product.CPU,
			"type":  "platform",
		}
		result["Year"] = map[string]any{
			"key":   "hw.year",
			"value": strconv.Itoa(product.Year),
			"type":  "platform",
		}
	}

	totalPhysical, totalLogical := platform.GetTotalCPU()
	if totalPhysical > 0 {
		result["CPU Physical Cores"] = map[string]any{
			"key":   "hw.physicalcpu",
			"value": strconv.Itoa(totalPhysical),
			"type":  "sysctl",
		}
	}

	if totalLogical > 0 {
		result["CPU Logical Cores"] = map[string]any{
			"key":   "hw.logicalcpu",
			"value": strconv.Itoa(totalLogical),
			"type":  "sysctl",
		}
	}

	for i, level := range platform.GetPerfLevels() {
		result[fmt.Sprintf("%s CPU Cores", level.Name)] = map[string]any{
			"key":   fmt.Sprintf("hw.perflevel%d.physicalcpu", i),
			"value": strconv.Itoa(level.PhysicalCPU),
			"type":  "sysctl",
		}
	}

	return result
}

// TODO replace with a variant from an utility package
// deepCopy copies all entries from src into dest via JSON round-trip, performing a deep clone.
func deepCopy(dest, src map[string]any) {
	jsonStr, _ := json.Marshal(src)
	_ = json.Unmarshal(jsonStr, &dest)
}

// TODO replace with a variant from an utility package
// merge returns a new map containing all entries from a and b, with b values taking precedence on conflicts.
func merge(a, b map[string]any) map[string]any {
	out := make(map[string]any)
	deepCopy(out, a)

	for k, v := range b {
		if v, ok := v.(map[string]any); ok {
			if bv, ok := out[k]; ok {
				if bv, ok := bv.(map[string]any); ok {
					out[k] = merge(bv, v)

					continue
				}
			}
		}

		out[k] = v
	}

	return out
}
