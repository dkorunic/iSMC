// SPDX-FileCopyrightText: Copyright (C) 2022 Roland Schaer
// SPDX-License-Identifier: GPL-3.0-only

//go:build darwin

package output

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/dkorunic/iSMC/hid"
	"github.com/dkorunic/iSMC/internal/platform"
	"github.com/dkorunic/iSMC/smc"
	"github.com/fvbommel/sortorder"
)

// Monkey-patching hooks for tests. Tests mutate these; never call t.Parallel() in output tests.
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

// isFloatType reports whether typ has a "quantity unit" string form that format() should split.
func isFloatType(typ string) bool {
	switch typ {
	case "flt", "ioft", hid.SensorType:
		return true
	}

	_, ok := smc.AppleFPConv[typ]

	return ok
}

// merge overlays entries from b onto a (b wins on conflicts) and returns a.
// a is mutated in place and must be owned exclusively by the caller; the sensor
// getters always pass freshly built maps, so no defensive clone is needed.
func merge(a, b map[string]any) map[string]any {
	for k, bVal := range b {
		if bMap, ok := bVal.(map[string]any); ok {
			if aVal, ok := a[k]; ok {
				if aMap, ok := aVal.(map[string]any); ok {
					a[k] = merge(aMap, bMap)

					continue
				}
			}
		}

		a[k] = bVal
	}

	return a
}
