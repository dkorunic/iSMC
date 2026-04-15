// Copyright (C) 2022  Dinko Korunic
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

package hid

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Foundation -framework IOKit

#import <Foundation/Foundation.h>
#import <IOKit/hidsystem/IOHIDEventSystemClient.h>
#include <unistd.h>

typedef struct __IOHIDEvent         *IOHIDEventRef;
typedef struct __IOHIDServiceClient *IOHIDServiceClientRef;
typedef double                      IOHIDFloat;

IOHIDEventSystemClientRef IOHIDEventSystemClientCreate(CFAllocatorRef allocator);

int IOHIDEventSystemClientSetMatching(IOHIDEventSystemClientRef client, CFDictionaryRef match);

IOHIDEventRef IOHIDServiceClientCopyEvent(IOHIDServiceClientRef, int64_t, int32_t, int64_t);

CFStringRef IOHIDServiceClientCopyProperty(IOHIDServiceClientRef service, CFStringRef property);

IOHIDFloat IOHIDEventGetFloatValue(IOHIDEventRef event, int32_t field);

#define IOHIDEventFieldBase(type) (type << 16)
#define kIOHIDEventTypeTemperature  15
#define kIOHIDEventTypePower        25

NSDictionary *matching(int page, int usage) {
    NSDictionary *dict = @{
        @"PrimaryUsagePage" : [NSNumber numberWithInt:page],
        @"PrimaryUsage" : [NSNumber numberWithInt:usage],
    };

    return dict;
}

// getNamesFromServices extracts the "Product" property from each service in srvRef.
// The caller is responsible for releasing the returned NSArray.
static NSArray *getNamesFromServices(CFArrayRef srvRef) {
    NSArray         *srvs = (__bridge NSArray *)srvRef;
    long            count = [srvs count];
    NSMutableArray  *array = [[NSMutableArray alloc] init];

    for (int i = 0; i < count; i++) {
        IOHIDServiceClientRef   sc = (IOHIDServiceClientRef)srvs[i];
        NSString                *name = (NSString *)IOHIDServiceClientCopyProperty(sc, (__bridge CFStringRef)@"Product");

        if (name) {
            [array addObject:name];
            [name release];
        } else {
            [array addObject:@"noname"];
        }
    }

    return array;
}

// getPowerValuesFromServices reads the power event float value from each service in srvRef.
// The caller is responsible for releasing the returned NSArray.
static NSArray *getPowerValuesFromServices(CFArrayRef srvRef) {
    NSArray         *srvs = (__bridge NSArray *)srvRef;
    long            count = [srvs count];
    NSMutableArray  *array = [[NSMutableArray alloc] init];

    for (int i = 0; i < count; i++) {
        IOHIDServiceClientRef   sc = (IOHIDServiceClientRef)srvs[i];
        IOHIDEventRef           event = IOHIDServiceClientCopyEvent(sc, kIOHIDEventTypePower, 0, 0);

        double temp = 0.0;

        if (event != 0) {
            temp = IOHIDEventGetFloatValue(event, IOHIDEventFieldBase(kIOHIDEventTypePower)) / 1000.0;
            CFRelease(event);
        }

        [array addObject:[NSNumber numberWithDouble:temp]];
    }

    return array;
}

// getThermalValuesFromServices reads the temperature event float value from each service in srvRef.
// The caller is responsible for releasing the returned NSArray.
static NSArray *getThermalValuesFromServices(CFArrayRef srvRef) {
    NSArray         *srvs = (__bridge NSArray *)srvRef;
    long            count = [srvs count];
    NSMutableArray  *array = [[NSMutableArray alloc] init];

    for (int i = 0; i < count; i++) {
        IOHIDServiceClientRef   sc = (IOHIDServiceClientRef)srvs[i];
        IOHIDEventRef           event = IOHIDServiceClientCopyEvent(sc, kIOHIDEventTypeTemperature, 0, 0);

        double temp = 0.0;

        if (event != 0) {
            temp = IOHIDEventGetFloatValue(event, IOHIDEventFieldBase(kIOHIDEventTypeTemperature));
            CFRelease(event);
        }

        [array addObject:[NSNumber numberWithDouble:temp]];
    }

    return array;
}

static NSString *dumpNamesValues(NSArray *kvsN, NSArray *kvsV) {
    NSMutableString *valueString = [[NSMutableString alloc] init];
    int             count = (int)MIN([kvsN count], [kvsV count]);

    for (int i = 0; i < count; i++) {
        @autoreleasepool {
            NSString *name = kvsN[i];
            double   value = [kvsV[i] doubleValue];

            if (value <= 0.0) continue;

            NSString *output = [NSString stringWithFormat:@"%s:%lf\n", [name UTF8String], value];
            [valueString appendString:output];
        }
    }

    return valueString;
}

// kSP78RawThreshold is the boundary above which a tdev sensor reading is
// unambiguously a raw sp78 value (°C × 256) rather than a converted °C value.
// The maximum plausible converted Apple Silicon temperature is ~130°C (junction limit).
#define kSP78RawThreshold 130.0

// dumpThermalNamesValues formats thermal sensor names and values, applying sp78 conversion
// for specific HID thermal sensors that use the sp78 fixed-point format (e.g., PMU tdev sensors)
static NSString *dumpThermalNamesValues(NSArray *kvsN, NSArray *kvsV) {
    NSMutableString *valueString = [[NSMutableString alloc] init];
    int             count = (int)MIN([kvsN count], [kvsV count]);

    for (int i = 0; i < count; i++) {
        @autoreleasepool {
            NSString *name = kvsN[i];
            double   value = [kvsV[i] doubleValue];

            if (value <= 0.0) continue;

            // PMU tdev sensors (e.g., "PMU tdev1") report temperatures in sp78 fixed-point
            // format (raw = °C × 256, e.g. 6400.0 for 25°C). Values above kSP78RawThreshold
            // are unambiguously raw sp78 — see kSP78RawThreshold definition for reasoning.
            NSRange range = [name rangeOfString:@"tdev"];
            if (range.location != NSNotFound) {
                if (range.location + 4 < [name length]) {
                    unichar nextChar = [name characterAtIndex:range.location + 4];
                    if (value > kSP78RawThreshold && nextChar >= '1' && nextChar <= '9') {
                        value = value / 256.0;
                    }
                }
            }

            NSString *output = [NSString stringWithFormat:@"%s:%lf\n", [name UTF8String], value];
            [valueString appendString:output];
        }
    }

    return valueString;
}

// queryHIDPowerSensors queries IOHIDEventSystem for power sensors matching the given
// HID page and usage, returning a "name:value\n" formatted string. The caller must free it.
static char *queryHIDPowerSensors(int page, int usage) {
    char *finalStr = strdup("");

    @autoreleasepool {
        IOHIDEventSystemClientRef system = IOHIDEventSystemClientCreate(kCFAllocatorDefault);

        if (system) {
            NSDictionary    *sensors = matching(page, usage);
            IOHIDEventSystemClientSetMatching(system, (__bridge CFDictionaryRef)sensors);
            CFArrayRef      srvRef = IOHIDEventSystemClientCopyServices(system);
            CFRelease(system);

            if (srvRef) {
                NSArray     *names = getNamesFromServices(srvRef);
                NSArray     *values = getPowerValuesFromServices(srvRef);
                NSString    *result = dumpNamesValues(names, values);
                CFRelease(srvRef);

                const char  *utf8 = result ? [result UTF8String] : "";
                free(finalStr);
                finalStr = strdup(utf8 ? utf8 : "");

                CFRelease(names);
                CFRelease(values);
                CFRelease(result);
            }
        }
    }

    return finalStr;
}

char *getCurrents() {
    return queryHIDPowerSensors(0xff08, 2);
}

char *getVoltages() {
    return queryHIDPowerSensors(0xff08, 3);
}

char *getThermals() {
    char *finalStr = strdup("");

    @autoreleasepool {
        IOHIDEventSystemClientRef system = IOHIDEventSystemClientCreate(kCFAllocatorDefault);

        if (system) {
            NSDictionary    *sensors = matching(0xff00, 5);
            IOHIDEventSystemClientSetMatching(system, (__bridge CFDictionaryRef)sensors);
            CFArrayRef      srvRef = IOHIDEventSystemClientCopyServices(system);
            CFRelease(system);

            if (srvRef) {
                NSArray     *names = getNamesFromServices(srvRef);
                NSArray     *values = getThermalValuesFromServices(srvRef);
                NSString    *result = dumpThermalNamesValues(names, values);
                CFRelease(srvRef);

                const char  *utf8 = result ? [result UTF8String] : "";
                free(finalStr);
                finalStr = strdup(utf8 ? utf8 : "");

                CFRelease(names);
                CFRelease(values);
                CFRelease(result);
            }
        }
    }

    return finalStr;
}

// HIDSensorData holds strdup'd C strings for all three sensor types. The caller
// must free each non-NULL field.
typedef struct {
    char *currents;
    char *voltages;
    char *thermals;
} HIDSensorData;

// getAllHIDSensors opens a single HID event system client, queries all three
// sensor types by updating the matching criteria between queries, and returns
// their formatted output strings. Each field is always a valid strdup'd string
// (never NULL) that the caller must free.
HIDSensorData getAllHIDSensors(void) {
    HIDSensorData result = { strdup(""), strdup(""), strdup("") };

    @autoreleasepool {
        IOHIDEventSystemClientRef system = IOHIDEventSystemClientCreate(kCFAllocatorDefault);

        if (system) {
        // Currents (page 0xff08, usage 2)
        {
            NSDictionary    *sensors = matching(0xff08, 2);
            IOHIDEventSystemClientSetMatching(system, (__bridge CFDictionaryRef)sensors);
            CFArrayRef      srvRef = IOHIDEventSystemClientCopyServices(system);

            if (srvRef) {
                NSArray     *names = getNamesFromServices(srvRef);
                NSArray     *values = getPowerValuesFromServices(srvRef);
                NSString    *str = dumpNamesValues(names, values);
                const char  *utf8 = str ? [str UTF8String] : "";
                free(result.currents);
                result.currents = strdup(utf8 ? utf8 : "");
                CFRelease(names);
                CFRelease(values);
                CFRelease(str);
                CFRelease(srvRef);
            }
        }

        // Voltages (page 0xff08, usage 3)
        {
            NSDictionary    *sensors = matching(0xff08, 3);
            IOHIDEventSystemClientSetMatching(system, (__bridge CFDictionaryRef)sensors);
            CFArrayRef      srvRef = IOHIDEventSystemClientCopyServices(system);

            if (srvRef) {
                NSArray     *names = getNamesFromServices(srvRef);
                NSArray     *values = getPowerValuesFromServices(srvRef);
                NSString    *str = dumpNamesValues(names, values);
                const char  *utf8 = str ? [str UTF8String] : "";
                free(result.voltages);
                result.voltages = strdup(utf8 ? utf8 : "");
                CFRelease(names);
                CFRelease(values);
                CFRelease(str);
                CFRelease(srvRef);
            }
        }

        // Thermals (page 0xff00, usage 5)
        {
            NSDictionary    *sensors = matching(0xff00, 5);
            IOHIDEventSystemClientSetMatching(system, (__bridge CFDictionaryRef)sensors);
            CFArrayRef      srvRef = IOHIDEventSystemClientCopyServices(system);

            if (srvRef) {
                NSArray     *names = getNamesFromServices(srvRef);
                NSArray     *values = getThermalValuesFromServices(srvRef);
                NSString    *str = dumpThermalNamesValues(names, values);
                const char  *utf8 = str ? [str UTF8String] : "";
                free(result.thermals);
                result.thermals = strdup(utf8 ? utf8 : "");
                CFRelease(names);
                CFRelease(values);
                CFRelease(str);
                CFRelease(srvRef);
            }
        }

        CFRelease(system);
        }   // closes if (system)
    }       // closes @autoreleasepool

    return result;
}
*/
import "C"

import (
	"unsafe"
)

// GetAll returns all detected HID sensor results using a single HID client session.
func GetAll() map[string]any {
	data := C.getAllHIDSensors()
	sensors := make(map[string]any)

	sensors["Current"] = hidGet(data.currents, "A")
	sensors["Temperature"] = hidGet(data.thermals, "°C")
	sensors["Voltage"] = hidGet(data.voltages, "V")

	return sensors
}

// hidGet converts a C string returned by an HID sensor function into a sensor map,
// freeing the C string before returning. Returns an empty map when cStr is nil.
func hidGet(cStr *C.char, unit string) map[string]any {
	if cStr == nil {
		return map[string]any{}
	}

	defer C.free(unsafe.Pointer(cStr)) //nolint:wsl,nlreturn

	return getGeneric(unit, cStr)
}

// GetCurrent returns detected HID current sensor results.
func GetCurrent() map[string]any {
	return hidGet(C.getCurrents(), "A")
}

// GetVoltage returns detected HID voltage sensor results.
func GetVoltage() map[string]any {
	return hidGet(C.getVoltages(), "V")
}

// GetTemperature returns detected HID temperature sensor results.
func GetTemperature() map[string]any {
	return hidGet(C.getThermals(), "°C")
}
