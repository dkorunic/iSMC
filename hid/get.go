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

NSDictionary *matching(int page, int usage) {
    NSDictionary *dict = @{
        @"PrimaryUsagePage" : [NSNumber numberWithInt:page],
        @"PrimaryUsage" : [NSNumber numberWithInt:usage],
    };

    return dict;
}

NSArray *getProductNames(NSDictionary *sensors) {
    IOHIDEventSystemClientRef system = IOHIDEventSystemClientCreate(kCFAllocatorDefault);

    if (!system) return [[NSArray alloc] init];

    IOHIDEventSystemClientSetMatching(system, (__bridge CFDictionaryRef)sensors);
    CFArrayRef matchingsrvsRef = IOHIDEventSystemClientCopyServices(system);
    NSArray *matchingsrvs = (__bridge NSArray *)matchingsrvsRef;

    long            count = [matchingsrvs count];
    NSMutableArray  *array = [[NSMutableArray alloc] init];

    for (int i = 0; i < count; i++) {
        IOHIDServiceClientRef   sc = (IOHIDServiceClientRef)matchingsrvs[i];
        NSString                *name = (NSString *)IOHIDServiceClientCopyProperty(sc, (__bridge CFStringRef)@"Product");

        if (name) {
            [array addObject:name];
            [name release];
        } else {
            [array addObject:@"noname"];
        }
    }

    if (matchingsrvsRef) CFRelease(matchingsrvsRef);
    CFRelease(system);

    return array;
}

#define IOHIDEventFieldBase(type) (type << 16)
#define kIOHIDEventTypeTemperature  15
#define kIOHIDEventTypePower        25

NSArray *getPowerValues(NSDictionary *sensors) {
    IOHIDEventSystemClientRef system = IOHIDEventSystemClientCreate(kCFAllocatorDefault);

    if (!system) return [[NSArray alloc] init];

    IOHIDEventSystemClientSetMatching(system, (__bridge CFDictionaryRef)sensors);
    CFArrayRef matchingsrvsRef = IOHIDEventSystemClientCopyServices(system);
    NSArray *matchingsrvs = (__bridge NSArray *)matchingsrvsRef;

    long            count = [matchingsrvs count];
    NSMutableArray  *array = [[NSMutableArray alloc] init];

    for (int i = 0; i < count; i++) {
        IOHIDServiceClientRef   sc = (IOHIDServiceClientRef)matchingsrvs[i];
        IOHIDEventRef           event = IOHIDServiceClientCopyEvent(sc, kIOHIDEventTypePower, 0, 0);

        NSNumber    *value;
        double      temp = 0.0;

        if (event != 0) {
            temp = IOHIDEventGetFloatValue(event, IOHIDEventFieldBase(kIOHIDEventTypePower)) / 1000.0;
            CFRelease(event);
        }

        value = [NSNumber numberWithDouble:temp];
        [array addObject:value];
    }

    if (matchingsrvsRef) CFRelease(matchingsrvsRef);
    CFRelease(system);

    return array;
}

NSArray *getThermalValues(NSDictionary *sensors) {
    IOHIDEventSystemClientRef system = IOHIDEventSystemClientCreate(kCFAllocatorDefault);

    if (!system) return [[NSArray alloc] init];

    IOHIDEventSystemClientSetMatching(system, (__bridge CFDictionaryRef)sensors);
    CFArrayRef matchingsrvsRef = IOHIDEventSystemClientCopyServices(system);
    NSArray *matchingsrvs = (__bridge NSArray *)matchingsrvsRef;

    long            count = [matchingsrvs count];
    NSMutableArray  *array = [[NSMutableArray alloc] init];

    for (int i = 0; i < count; i++) {
        IOHIDServiceClientRef   sc = (IOHIDServiceClientRef)matchingsrvs[i];
        IOHIDEventRef           event = IOHIDServiceClientCopyEvent(sc, kIOHIDEventTypeTemperature, 0, 0);

        NSNumber    *value;
        double      temp = 0.0;

        if (event != 0) {
            temp = IOHIDEventGetFloatValue(event, IOHIDEventFieldBase(kIOHIDEventTypeTemperature));
            CFRelease(event);
        }

        value = [NSNumber numberWithDouble:temp];
        [array addObject:value];
    }

    if (matchingsrvsRef) CFRelease(matchingsrvsRef);
    CFRelease(system);

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
            // format (raw = °C × 256, e.g. 6400.0 for 25°C). Apple Silicon thermal shutdown
            // occurs at ~100-110°C and the junction temperature maximum is ~125°C, so a
            // legitimate converted reading never exceeds ~130°C. The minimum raw sp78 encoding
            // for 1°C is 256. Values above 130.0 are unambiguously raw sp78 — this threshold
            // avoids the false positive at exactly 256.0 (= 1°C in sp78) from the old guard.
            NSRange range = [name rangeOfString:@"tdev"];
            if (range.location != NSNotFound) {
                if (range.location + 4 < [name length]) {
                    unichar nextChar = [name characterAtIndex:range.location + 4];
                    if (value > 130.0 && nextChar >= '1' && nextChar <= '9') {
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

char *getCurrents() {
    char *finalStr;

    @autoreleasepool {
        NSDictionary    *currentSensors = matching(0xff08, 2);
        NSArray         *currentNames = getProductNames(currentSensors);
        NSArray         *currentValues = getPowerValues(currentSensors);
        NSString        *result = dumpNamesValues(currentNames, currentValues);
        const char      *utf8 = result ? [result UTF8String] : "";
        finalStr = strdup(utf8 ? utf8 : "");

        CFRelease(currentNames);
        CFRelease(currentValues);
        CFRelease(result);
    }

    return finalStr;
}

char *getVoltages() {
    char *finalStr;

    @autoreleasepool {
        NSDictionary    *voltageSensors = matching(0xff08, 3);
        NSArray         *voltageNames = getProductNames(voltageSensors);
        NSArray         *voltageValues = getPowerValues(voltageSensors);
        NSString        *result = dumpNamesValues(voltageNames, voltageValues);
        const char      *utf8 = result ? [result UTF8String] : "";
        finalStr = strdup(utf8 ? utf8 : "");

        CFRelease(voltageNames);
        CFRelease(voltageValues);
        CFRelease(result);
    }

    return finalStr;
}

char *getThermals() {
    char *finalStr;

    @autoreleasepool {
        NSDictionary    *thermalSensors = matching(0xff00, 5);
        NSArray         *thermalNames = getProductNames(thermalSensors);
        NSArray         *thermalValues = getThermalValues(thermalSensors);
        NSString        *result = dumpThermalNamesValues(thermalNames, thermalValues);
        const char      *utf8 = result ? [result UTF8String] : "";
        finalStr = strdup(utf8 ? utf8 : "");

        CFRelease(thermalNames);
        CFRelease(thermalValues);
        CFRelease(result);
    }

    return finalStr;
}
*/
import "C"

import (
	"unsafe"
)

// GetAll returns all detected HID sensor results.
func GetAll() map[string]any {
	sensors := make(map[string]any)

	sensors["Current"] = GetCurrent()
	sensors["Temperature"] = GetTemperature()
	sensors["Voltage"] = GetVoltage()

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
