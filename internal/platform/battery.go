// SPDX-FileCopyrightText: Copyright (C) 2026  Dinko Korunic
// SPDX-License-Identifier: GPL-3.0-only

//go:build darwin

package platform

/*
#cgo LDFLAGS: -framework IOKit
#cgo LDFLAGS: -framework CoreFoundation
#include <stdlib.h>
#include <IOKit/IOKitLib.h>
#include <CoreFoundation/CoreFoundation.h>

static io_service_t battery_open(void) {
    return IOServiceGetMatchingService(kIOMainPortDefault, IOServiceMatching("AppleSmartBattery"));
}

static void battery_close(io_service_t s) { IOObjectRelease(s); }

// battery_int reads a numeric or boolean AppleSmartBattery property; 0 on success.
static int battery_int(io_service_t s, const char *name, int64_t *out) {
    CFStringRef key = CFStringCreateWithCString(NULL, name, kCFStringEncodingUTF8);
    if (!key) return -1;
    CFTypeRef v = IORegistryEntryCreateCFProperty(s, key, kCFAllocatorDefault, 0);
    CFRelease(key);
    if (!v) return -1;
    int rc = -1;
    if (CFGetTypeID(v) == CFNumberGetTypeID() && CFNumberGetValue((CFNumberRef)v, kCFNumberSInt64Type, out)) {
        rc = 0;
    } else if (CFGetTypeID(v) == CFBooleanGetTypeID()) {
        *out = CFBooleanGetValue((CFBooleanRef)v);
        rc = 0;
    }
    CFRelease(v);
    return rc;
}
*/
import "C"

import (
	"fmt"
	"strconv"
	"unsafe"
)

// batteryType is the "type" of battery rows read from the IORegistry.
const batteryType = "ioreg"

// Battery returns AppleSmartBattery details from the IORegistry (no root
// needed), keyed like the SMC battery rows. Macs without a battery (no service,
// or a stub with BatteryInstalled false) get an empty map. Battery temperature is left to the SMC TB* sensors.
func Battery() map[string]any {
	s := C.battery_open()
	if s == 0 {
		return map[string]any{}
	}
	defer C.battery_close(s)

	return batteryRows(batteryService(s).get)
}

// batteryService is an open AppleSmartBattery IORegistry entry.
type batteryService C.io_service_t

// get reads one numeric or boolean property of the battery service.
func (s batteryService) get(name string) (int64, bool) {
	cname := C.CString(name)
	defer C.free(unsafe.Pointer(cname))

	var v C.int64_t

	ok := C.battery_int(C.io_service_t(s), cname, &v) == 0

	return int64(v), ok
}

// batteryRows builds the battery rows from get, which reads one property.
// Units: capacities in mAh, Voltage in mV, Amperage in mA (negative while
// discharging). CurrentCapacity and MaxCapacity are percent on Apple Silicon
// and mAh on Intel, so their ratio is the charge level on both.
func batteryRows(get func(string) (int64, bool)) map[string]any {
	out := make(map[string]any)

	// Desktops (Mac Studio, mini) publish a zero-filled AppleSmartBattery stub.
	if installed, ok := get("BatteryInstalled"); !ok || installed == 0 {
		return out
	}
	row := func(desc, key, value string) {
		out[desc] = map[string]any{"key": key, "value": value, "type": batteryType}
	}

	if v, ok := get("CycleCount"); ok {
		row("Battery Cycle Count", "CycleCount", strconv.FormatInt(v, 10))
	}

	if cur, ok := get("CurrentCapacity"); ok {
		if maxCap, ok := get("MaxCapacity"); ok && maxCap > 0 {
			row("Battery Charge", "CurrentCapacity", fmt.Sprintf("%d %%", cur*100/maxCap))
		}
	}

	design, hasDesign := get("DesignCapacity")
	if hasDesign {
		row("Battery Design Capacity", "DesignCapacity", fmt.Sprintf("%d mAh", design))
	}

	// NominalChargeCapacity is what System Information reports; older firmware
	// only has AppleRawMaxCapacity.
	for _, key := range []string{"NominalChargeCapacity", "AppleRawMaxCapacity"} {
		full, ok := get(key)
		if !ok || full <= 0 {
			continue
		}

		row("Battery Full Charge Capacity", key, fmt.Sprintf("%d mAh", full))

		if hasDesign && design > 0 {
			row("Battery Health", key, fmt.Sprintf("%d %%", full*100/design))
		}

		break
	}

	if v, ok := get("Voltage"); ok {
		row("Battery Voltage", "Voltage", fmt.Sprintf("%.3f V", float64(v)/1000))
	}

	if v, ok := get("Amperage"); ok {
		row("Battery Amperage", "Amperage", fmt.Sprintf("%.3f A", float64(v)/1000))
	}

	if state, ok := batteryState(get); ok {
		row("Battery State", "IsCharging", state)
	}

	return out
}

// batteryState names the charge state from the FullyCharged, IsCharging and
// ExternalConnected flags.
func batteryState(get func(string) (int64, bool)) (string, bool) {
	ext, ok := get("ExternalConnected")
	if !ok {
		return "", false
	}

	if full, _ := get("FullyCharged"); full != 0 {
		return "Charged", true
	}

	if charging, _ := get("IsCharging"); charging != 0 {
		return "Charging", true
	}

	if ext != 0 {
		return "Not Charging", true
	}

	return "On Battery", true
}
