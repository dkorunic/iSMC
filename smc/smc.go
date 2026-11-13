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

	// Floor rejecting firmware sentinels from inactive slots (issue #37). The
	// margin is thin on both sides: junk reaches 14.81 °C (TV14, M5 Pro) and
	// genuine probes go down to 16.7 °C (TD02, M1 Pro in a cool room), so
	// machines in a room below ~15 °C lose real sensors. Recheck against
	// internal/reports before moving it.
	minTempCelsius = 15.0

	// Ceiling rejecting decoder/firmware garbage (issue #39); matches
	// rawTempMax in rawtemp.go.
	maxTempCelsius = 200.0
)

// Platform tag and CPU family sentinels used by filterForPlatform and platformMatches.
const (
	platformAll   = "All"
	familyApple   = "Apple"
	familyIntel   = "Intel"
	familyUnknown = "Unknown"

	// skuBase suffixes a family to tag the base SKU only ("M4 Base"), since the
	// base SKU's Product.CPU equals the family name and "M4" means every M4 SKU.
	skuBase = " Base"
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
		addFanMode(c, fans, i)

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

// Fan mode keys: F%dMd on M1-M4, lowercase F%dmd on M5 Max. Ftst ("force
// test") is the switch fan-control tools set on some M1-M4 Macs to take fans
// from thermalmonitord; it is absent elsewhere (M1 Ultra, M5 Max).
const (
	fanModeKey      = "F%dMd"
	fanModeKeyLower = "F%dmd"
	fanForceTestKey = "Ftst"
)

// addFanMode adds "Fan N Mode" for fan i, showing whether macOS or a
// fan-control tool drives it, and the Ftst switch once, when the Mac has it.
func addFanMode(c uint, fans map[string]any, i uint32) {
	for _, tpl := range []string{fanModeKey, fanModeKeyLower} {
		key := fmt.Sprintf(tpl, i)

		v, t, err := getKeyUint32(c, key)
		if err == nil {
			fans[fmt.Sprintf("Fan %d Mode", i+1)] = map[string]any{"key": key, "value": fanModeName(v), "type": t}

			break
		}
	}

	if i != 0 {
		return
	}

	v, t, err := getKeyUint32(c, fanForceTestKey)
	if err != nil {
		return
	}

	state := "off"
	if v != 0 {
		state = "on"
	}

	fans["Fan Force Test"] = map[string]any{"key": fanForceTestKey, "value": state, "type": t}
}

// fanModeName names a fan mode value: 0 auto (macOS), 1 manual (a fan-control
// tool holds the fan), 3 system. Unknown values are shown as numbers.
func fanModeName(v uint32) string {
	switch v {
	case 0:
		return "auto"
	case 1:
		return "manual"
	case 3:
		return "system"
	default:
		return strconv.FormatUint(uint64(v), 10)
	}
}

// getGenericSensors reads smcSlice over conn, expanding wildcard (%) keys to
// indices 0–9, and returns description → sensor entry formatted with unit.
func getGenericSensors(conn uint, unit string, smcSlice []SensorStat) map[string]any {
	generic := make(map[string]any)

	forEachKey(smcSlice, func(key, desc string) {
		addGeneric(generic, conn, key, desc, unit)
	})

	return generic
}

// forEachKey calls fn for every concrete (key, description) pair in rows, in
// row order, expanding a wildcard (%) key to indices 0–9 and its description
// to 1–10.
func forEachKey(rows []SensorStat, fn func(key, desc string)) {
	for _, v := range rows {
		if !strings.Contains(v.Key, KeyWildcard) {
			fn(v.Key, v.Desc)

			continue
		}

		for i := range 10 {
			fn(strings.Replace(v.Key, KeyWildcard, strconv.Itoa(i), 1),
				strings.Replace(v.Desc, KeyWildcard, strconv.Itoa(i+1), 1))
		}
	}
}

// isValidReading filters sentinel and garbage readings: near-zero/negative for
// any unit, plus temperatures outside the plausibility window.
func isValidReading(val float32, unit string) bool {
	if val < 0.005 {
		return false
	}

	if unit == TempUnit && (val < minTempCelsius || val > maxTempCelsius) {
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
// family and SKU (see platformMatches).
func filterForPlatform(smcSlice []SensorStat) []SensorStat {
	family, sku := resolvePlatform()

	return rowsFor(smcSlice, family, sku)
}

// rowsFor returns the rows of smcSlice whose Platform tag accepts family and
// sku (see platformMatches), in table order.
func rowsFor(smcSlice []SensorStat, family, sku string) []SensorStat {
	out := make([]SensorStat, 0, len(smcSlice))

	for _, v := range smcSlice {
		if platformMatches(v.Platform, family, sku) {
			out = append(out, v)
		}
	}

	return out
}

// resolvePlatform returns the chip family and SKU (Product.CPU) for the current
// host, falling back to the runtime architecture with no SKU when platform
// cannot identify the machine.
func resolvePlatform() (string, string) {
	if p, ok := platform.GetProduct(); ok {
		return p.Family, p.CPU
	}

	switch runtime.GOARCH {
	case "arm64":
		return familyApple, ""
	case "amd64", "386":
		return familyIntel, ""
	}

	return familyUnknown, ""
}

// platformMatches reports whether a row's Platform tag accepts family and sku
// (Product.CPU, may be empty). Unlike filterForPlatform it takes them
// explicitly, enabling offline cross-checks. "" and "All" match anything;
// "Apple" matches any Apple Silicon family; a family tag ("M4") matches every
// SKU of it; a SKU tag ("M4 Pro") matches that SKU; "<family> Base" matches
// only the base SKU.
func platformMatches(rowPlatform, family, sku string) bool {
	switch {
	case rowPlatform == "" || rowPlatform == platformAll || rowPlatform == family:
		return true
	case rowPlatform == familyApple:
		return strings.HasPrefix(family, "M") ||
			strings.HasPrefix(family, "A") ||
			family == familyApple
	case sku == "":
		return false
	case strings.HasSuffix(rowPlatform, skuBase):
		return rowPlatform == sku+skuBase
	}

	return rowPlatform == sku
}

var (
	tempDescCacheMu sync.Mutex
	tempDescCache   = map[string]map[string]string{}
)

// LookupTempDesc returns the AppleTemp description for a concrete key under
// family and sku (see platformMatches), matching GetTemperature's runtime
// resolution (last-write-wins on duplicates). The key→description map is built
// once per platform via MappedTempKeys and cached, so repeated guess-output
// lookups are O(1).
func LookupTempDesc(key, family, sku string) (string, bool) {
	tempDescCacheMu.Lock()

	m, ok := tempDescCache[family+"/"+sku]
	if !ok {
		m = MappedTempKeys(family, sku)
		tempDescCache[family+"/"+sku] = m
	}

	tempDescCacheMu.Unlock()

	desc, found := m[key]

	return desc, found
}

// MappedTempKeys returns every concrete temperature key in scope for family and
// sku (see platformMatches), wildcards expanded to 0–9, mapped to the
// description GetTemperature resolves.
func MappedTempKeys(family, sku string) map[string]string {
	out := make(map[string]string)

	forEachKey(rowsFor(AppleTemp, family, sku), func(key, desc string) { out[key] = desc })

	return out
}
