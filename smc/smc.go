// SPDX-FileCopyrightText: Copyright (C) 2019  Dinko Korunic
// SPDX-License-Identifier: GPL-3.0-only

//go:build darwin

package smc

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/dkorunic/iSMC/gosmc"
	"github.com/dkorunic/iSMC/internal/platform"
)

// errSMCOpen is returned when the Apple SMC connection cannot be opened; the
// IOKit return code is attached as dynamic context.
var errSMCOpen = errors.New("unable to open Apple SMC")

const (
	AppleSMC    = "AppleSMC"
	FanNum      = "FNum"
	BattNum     = "BNum"
	BattPwr     = "BATP"
	BattInf     = "BSIn"
	KeyWildcard = "%"

	// Guards against a corrupt/spoofed FNum key; real Macs have ≤8 fans.
	maxFans = 32

	TempUnit = "°C"

	// Rejects firmware sentinels from inactive sensor slots (observed: −4..5.2 °C).
	minTempCelsius = 10.0
)

// Platform tag and CPU family sentinels used by filterForPlatform and platformMatches.
const (
	platformAll   = "All"
	familyApple   = "Apple"
	familyIntel   = "Intel"
	familyUnknown = "Unknown"
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

// openSMC opens the Apple SMC; the caller must close the connection with gosmc.SMCClose.
func openSMC() (uint, error) {
	c, res := gosmc.SMCOpen(AppleSMC)
	if res != gosmc.IOReturnSuccess {
		return 0, fmt.Errorf("%w: return code %v", errSMCOpen, res)
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

// getBattery reads battery keys over c, omitting any key whose read fails: a zero
// value with empty type is indistinguishable from a real zero (e.g. a desktop Mac).
func getBattery(c uint) map[string]any {
	out := make(map[string]any)

	n, tyNum, err := getKeyUint32(c, BattNum)
	if err == nil {
		out["Battery Count"] = map[string]any{
			"key":   BattNum,
			"value": n,
			"type":  tyNum,
		}
	}

	i, tyInf, err := getKeyUint32(c, BattInf)
	if err == nil {
		out["Battery Info"] = map[string]any{
			"key":   BattInf,
			"value": i,
			"type":  tyInf,
		}
	}

	b, tyPwr, err := getKeyBool(c, BattPwr)
	if err == nil {
		out["Battery Power"] = map[string]any{
			"key":   BattPwr,
			"value": b,
			"type":  tyPwr,
		}
	}

	return out
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

	// Omit fan data on a failed count read rather than emit a phantom "Fan Count: 0".
	val, smcType, err := getKeyUint32(c, FanNum)
	if err != nil {
		return fans
	}

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

// getGenericSensors reads smcSlice over conn, expanding wildcard (%) keys to
// indices 0–9, and returns description → sensor entry formatted with unit.
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

// isValidReading reports whether val is plausible for unit. Values below 0.005
// (zero, near-zero, negative) are rejected; temperature readings below
// minTempCelsius are firmware sentinels from inactive slots (observed −4..5.2 °C
// on M4 Pro 14-core).
func isValidReading(val float32, unit string) bool {
	if val < 0.005 {
		return false
	}

	if unit == TempUnit && val < minTempCelsius {
		return false
	}

	return true
}

// addGeneric reads key over conn and stores a valid reading under desc in generic.
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

// filterForPlatform returns the sensors whose Platform tag matches the detected
// family. "All"/"" always match; "Apple" matches any Apple Silicon family. Falls
// back to runtime architecture when the model is unknown.
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
	if family != "" && family != familyUnknown {
		return family
	}

	switch runtime.GOARCH {
	case "arm64":
		return familyApple
	case "amd64", "386":
		return familyIntel
	}

	return family
}

// platformMatches reports whether a row's Platform tag accepts family. Unlike
// filterForPlatform it takes an explicit family, enabling offline cross-checks
// (e.g. cmd/guess against any tag). "" and "All" match anything; "Apple" matches
// any Apple Silicon family.
func platformMatches(rowPlatform, family string) bool {
	if rowPlatform == "" || rowPlatform == platformAll {
		return true
	}

	if rowPlatform == familyApple {
		return strings.HasPrefix(family, "M") ||
			strings.HasPrefix(family, "A") ||
			family == familyApple
	}

	return rowPlatform == family
}

var (
	tempDescCacheMu sync.Mutex
	tempDescCache   = map[string]map[string]string{}
)

// LookupTempDesc returns the AppleTemp description for a concrete key under family,
// matching GetTemperature's runtime resolution (last-write-wins on duplicates).
// The per-family key→description map is built once via MappedTempKeys and cached,
// so repeated guess-output lookups are O(1).
func LookupTempDesc(key, family string) (string, bool) {
	tempDescCacheMu.Lock()

	m, ok := tempDescCache[family]
	if !ok {
		m = MappedTempKeys(family)
		tempDescCache[family] = m
	}

	tempDescCacheMu.Unlock()

	desc, found := m[key]

	return desc, found
}

// MappedTempKeys returns every concrete temperature key in scope for family,
// wildcards expanded to 0–9, mapped to the description GetTemperature resolves.
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
