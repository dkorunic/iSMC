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
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"

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

	// maxFans is the upper bound on the number of fans to read from SMC.
	// A physical Mac never has more than ~8 fans; this guards against a corrupt/spoofed FNum key.
	maxFans = 32

	// TempUnit is the unit string used for temperature sensors.
	TempUnit = "°C"

	// minTempCelsius is the minimum plausible temperature (°C) for any SMC thermal
	// sensor on a running Mac. Values below this are firmware sentinels from inactive
	// or unimplemented sensor slots (observed: −4, 0, 2.2, 3.4, 5.2 °C) and must be
	// rejected to prevent overwriting valid readings from a higher-priority scheme.
	minTempCelsius = 10.0
)

// SensorStat is SMC key to description mapping.
type SensorStat struct {
	Key      string
	Desc     string
	Platform string
}

//go:generate ./gen-sensors.sh sensors.go

var (
	filteredTempOnce    sync.Once
	filteredTempSensors []SensorStat
)

// filteredTemp returns AppleTemp filtered for the current platform, cached after the first call.
func filteredTemp() []SensorStat {
	filteredTempOnce.Do(func() {
		filteredTempSensors = filterForPlatform(AppleTemp)
	})

	return filteredTempSensors
}

// openSMC opens the Apple SMC. Returns (connection, nil) on success or (0, error) on failure.
// The caller must close a successful connection with gosmc.SMCClose.
func openSMC() (uint, error) {
	c, res := gosmc.SMCOpen(AppleSMC)
	if res != gosmc.IOReturnSuccess {
		return 0, fmt.Errorf("unable to open Apple SMC: return code %v", res)
	}

	return c, nil
}

// GetAll returns all SMC sensor readings grouped by category (Battery, Current, Fans, Temperature, Power, Voltage).
func GetAll() map[string]any {
	c, err := openSMC()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return map[string]any{}
	}
	defer gosmc.SMCClose(c)

	sensors := make(map[string]any)
	sensors["Battery"] = getBattery(c)
	sensors["Current"] = getGenericSensors(c, "A", AppleCurrent)
	sensors["Fans"] = getFans(c)
	sensors["Temperature"] = getGenericSensors(c, TempUnit, filteredTemp())
	sensors["Power"] = getGenericSensors(c, "W", ApplePower)
	sensors["Voltage"] = getGenericSensors(c, "V", AppleVoltage)

	return sensors
}

// GetBattery returns battery count, status flags, and AC power state read from SMC keys.
func GetBattery() map[string]any {
	c, err := openSMC()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return map[string]any{}
	}
	defer gosmc.SMCClose(c)

	return getBattery(c)
}

// getBattery reads battery keys using the provided SMC connection.
func getBattery(c uint) map[string]any {
	n, ty1, _ := getKeyUint32(c, BattNum) // Get number of batteries
	i, ty2, _ := getKeyUint32(c, BattInf) // Get battery info (needs bit decoding)
	b, ty3, _ := getKeyBool(c, BattPwr)   // Get AC status

	return map[string]any{
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
}

// GetCurrent returns current sensor readings (in amperes) from SMC.
func GetCurrent() map[string]any {
	c, err := openSMC()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return map[string]any{}
	}
	defer gosmc.SMCClose(c)

	return getGenericSensors(c, "A", AppleCurrent)
}

// GetFans returns fan count and per-fan speed readings (in RPM) from SMC.
func GetFans() map[string]any {
	c, err := openSMC()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return map[string]any{}
	}
	defer gosmc.SMCClose(c)

	return getFans(c)
}

// getFans reads fan sensors using the provided SMC connection.
func getFans(c uint) map[string]any {
	fans := make(map[string]any)

	val, smcType, _ := getKeyUint32(c, FanNum) // Get number of fans
	val = min(val, maxFans)

	fans["Fan Count"] = map[string]any{
		"key":   FanNum,
		"value": val,
		"type":  smcType,
	}

	for i := range val {
		for _, v := range AppleFans {
			key := fmt.Sprintf(v.Key, i)
			desc := fmt.Sprintf(v.Desc, i+1)

			fval, ftype, err := getKeyFloat32(c, key)
			if err != nil {
				continue
			}

			if isValidReading(fval, "rpm") {
				fans[desc] = map[string]any{
					"key":   key,
					"value": fmt.Sprintf("%4.0f rpm", fval),
					"type":  ftype,
				}
			}
		}
	}

	return fans
}

// getGenericSensors reads each sensor in smcSlice from SMC using conn, expanding
// wildcard keys (%) to indices 0–9, and returns a map of description → sensor entry
// formatted with the given unit string.
func getGenericSensors(conn uint, unit string, smcSlice []SensorStat) map[string]any {
	generic := make(map[string]any)

	for _, v := range smcSlice {
		key := v.Key
		desc := v.Desc

		if strings.IndexByte(key, KeyWildcard[0]) < 0 {
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

// isValidReading reports whether val is a plausible sensor reading for the given unit.
// It rejects any value below 0.005, which covers zero, near-zero, and all
// non-positive readings (float32 comparison against a positive threshold
// implicitly excludes negatives). For temperature sensors (unit == TempUnit) it
// additionally rejects readings below minTempCelsius, which are firmware
// sentinel values from inactive or unimplemented sensor slots (observed: −4,
// 2.2, 3.4, 5.2 °C on M4 Pro 14-core).
func isValidReading(val float32, unit string) bool {
	if val < 0.005 {
		return false
	}

	if unit == TempUnit && val < minTempCelsius {
		return false
	}

	return true
}

// addGeneric reads a single SMC key and adds the result to generic under desc if the value is
// valid according to isValidReading.
func addGeneric(generic map[string]any, conn uint, key, desc, unit string) {
	val, smcType, err := getKeyFloat32(conn, key)
	if err != nil {
		return
	}

	if isValidReading(val, unit) {
		generic[desc] = map[string]any{
			"key":   key,
			"value": fmt.Sprintf("%g %s", val, unit),
			"type":  smcType,
		}
	}
}

// GetPower returns power sensor readings (in watts) from SMC.
func GetPower() map[string]any {
	c, err := openSMC()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return map[string]any{}
	}
	defer gosmc.SMCClose(c)

	return getGenericSensors(c, "W", ApplePower)
}

// GetTemperature returns temperature sensor readings (in °C) from SMC, filtered to the detected platform family.
func GetTemperature() map[string]any {
	c, err := openSMC()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return map[string]any{}
	}
	defer gosmc.SMCClose(c)

	return getGenericSensors(c, TempUnit, filteredTemp())
}

// GetVoltage returns voltage sensor readings (in volts) from SMC.
func GetVoltage() map[string]any {
	c, err := openSMC()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return map[string]any{}
	}
	defer gosmc.SMCClose(c)

	return getGenericSensors(c, "V", AppleVoltage)
}

// filterForPlatform returns the subset of smcSlice whose Platform tag matches the detected hardware
// family (e.g. "M1", "Intel"). Sensors tagged "All" or "" are always included; sensors tagged
// "Apple" are included for any Apple Silicon family. Falls back to runtime architecture when the
// model cannot be identified.
func filterForPlatform(smcSlice []SensorStat) []SensorStat {
	family := resolveFamily()
	filteredSensors := make([]SensorStat, 0, len(smcSlice))

	for _, v := range smcSlice {
		if platformMatches(v.Platform, family) {
			filteredSensors = append(filteredSensors, v)
		}
	}

	return filteredSensors
}

// resolveFamily returns the chip family for the current host, falling back to
// the runtime architecture when platform.GetFamily cannot identify the model.
func resolveFamily() string {
	family := platform.GetFamily()
	if family != "" && family != "Unknown" {
		return family
	}

	switch runtime.GOARCH {
	case "arm64":
		return "Apple"
	case "amd64", "386":
		return "Intel"
	}

	return family
}

// platformMatches reports whether a SensorStat row's Platform tag accepts the
// given chip family. Mirrors the per-row scope of filterForPlatform but operates
// on a caller-supplied family rather than querying platform.GetFamily, so it can
// be used for offline cross-checks (e.g. cmd/guess against any family tag).
//
//	rowPlatform == "" || "All": always matches.
//	rowPlatform == "Apple":     matches any Apple Silicon family ("M*", "A*", or "Apple").
//	rowPlatform == family:      exact family match.
func platformMatches(rowPlatform, family string) bool {
	if rowPlatform == "" || rowPlatform == "All" {
		return true
	}

	if rowPlatform == "Apple" {
		return strings.HasPrefix(family, "M") ||
			strings.HasPrefix(family, "A") ||
			family == "Apple"
	}

	return rowPlatform == family
}

// LookupTempDesc returns the canonical description from the AppleTemp table for
// a single concrete SMC key, with the row's Platform column filtered against
// family. Wildcard rows are expanded to digits 0–9 to mirror getGenericSensors.
//
// The returned description matches what GetTemperature would produce at runtime
// (last-write-wins on duplicate (Key, Platform) entries). Useful for annotating
// guess-command output with the description that already exists in src/temp.txt.
func LookupTempDesc(key, family string) (string, bool) {
	var (
		desc  string
		found bool
	)

	for _, s := range AppleTemp {
		if !platformMatches(s.Platform, family) {
			continue
		}

		if !strings.Contains(s.Key, KeyWildcard) {
			if s.Key == key {
				desc, found = s.Desc, true
			}

			continue
		}

		for i := range 10 {
			iKey := strings.Replace(s.Key, KeyWildcard, strconv.Itoa(i), 1)
			if iKey == key {
				desc = strings.Replace(s.Desc, KeyWildcard, strconv.Itoa(i+1), 1)
				found = true
			}
		}
	}

	return desc, found
}

// MappedTempKeys returns every concrete SMC temperature key mapped in the
// AppleTemp table that is in scope for the given chip family. Wildcards are
// expanded to digits 0–9; the returned map mirrors the resolved descriptions
// emitted by GetTemperature for that family.
func MappedTempKeys(family string) map[string]string {
	out := make(map[string]string)

	for _, s := range AppleTemp {
		if !platformMatches(s.Platform, family) {
			continue
		}

		if !strings.Contains(s.Key, KeyWildcard) {
			out[s.Key] = s.Desc

			continue
		}

		for i := range 10 {
			iKey := strings.Replace(s.Key, KeyWildcard, strconv.Itoa(i), 1)
			iDesc := strings.Replace(s.Desc, KeyWildcard, strconv.Itoa(i+1), 1)
			out[iKey] = iDesc
		}
	}

	return out
}
