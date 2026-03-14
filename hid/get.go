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

    IOHIDEventSystemClientSetMatching(system, (__bridge CFDictionaryRef)sensors);
    NSArray *matchingsrvs = (__bridge NSArray *)IOHIDEventSystemClientCopyServices(system);

    long            count = [matchingsrvs count];
    NSMutableArray  *array = [[NSMutableArray alloc] init];

    for (int i = 0; i < count; i++) {
        IOHIDServiceClientRef   sc = (IOHIDServiceClientRef)matchingsrvs[i];
        NSString                *name = (NSString *)IOHIDServiceClientCopyProperty(sc, (__bridge CFStringRef)@"Product");

        if (name) {
            [array addObject:name];
        } else {
            [array addObject:@"noname"];
        }
    }

    return array;
}

#define IOHIDEventFieldBase(type) (type << 16)
#define kIOHIDEventTypeTemperature  15
#define kIOHIDEventTypePower        25

NSArray *getPowerValues(NSDictionary *sensors) {
    IOHIDEventSystemClientRef system = IOHIDEventSystemClientCreate(kCFAllocatorDefault);

    IOHIDEventSystemClientSetMatching(system, (__bridge CFDictionaryRef)sensors);
    NSArray *matchingsrvs = (NSArray *)IOHIDEventSystemClientCopyServices(system);

    long            count = [matchingsrvs count];
    NSMutableArray  *array = [[NSMutableArray alloc] init];

    for (int i = 0; i < count; i++) {
        IOHIDServiceClientRef   sc = (IOHIDServiceClientRef)matchingsrvs[i];
        IOHIDEventRef           event = IOHIDServiceClientCopyEvent(sc, kIOHIDEventTypePower, 0, 0);

        NSNumber    *value;
        double      temp = 0.0;

        if (event != 0) {
            temp = IOHIDEventGetFloatValue(event, IOHIDEventFieldBase(kIOHIDEventTypePower)) / 1000.0;
        }

        value = [NSNumber numberWithDouble:temp];
        [array addObject:value];
    }

    return array;
}

NSArray *getThermalValues(NSDictionary *sensors) {
    IOHIDEventSystemClientRef system = IOHIDEventSystemClientCreate(kCFAllocatorDefault);

    IOHIDEventSystemClientSetMatching(system, (__bridge CFDictionaryRef)sensors);
    NSArray *matchingsrvs = (__bridge NSArray *)IOHIDEventSystemClientCopyServices(system);

    long            count = [matchingsrvs count];
    NSMutableArray  *array = [[NSMutableArray alloc] init];

    for (int i = 0; i < count; i++) {
        IOHIDServiceClientRef   sc = (IOHIDServiceClientRef)matchingsrvs[i];
        IOHIDEventRef           event = IOHIDServiceClientCopyEvent(sc, kIOHIDEventTypeTemperature, 0, 0);

        NSNumber    *value;
        double      temp = 0.0;

        if (event != 0) {
            temp = IOHIDEventGetFloatValue(event, IOHIDEventFieldBase(kIOHIDEventTypeTemperature));
        }

        value = [NSNumber numberWithDouble:temp];
        [array addObject:value];
    }

    return array;
}

NSString *dumpNamesValues(NSArray *kvsN, NSArray *kvsV) {
    NSMutableString *valueString = [[NSMutableString alloc] init];
    int             count = [kvsN count];

    for (int i = 0; i < count; i++) {
        NSString *name = kvsN[i];
        double   value = [kvsV[i] doubleValue];

        NSString *output = [NSString stringWithFormat:@"%s:%lf\n", [name UTF8String], value];
        [valueString appendString:output];
    }

    return valueString;
}

// dumpThermalNamesValues formats thermal sensor names and values, applying sp78 conversion
// for specific HID thermal sensors that use the sp78 fixed-point format (e.g., PMU tdev sensors)
static NSString *dumpThermalNamesValues(NSArray *kvsN, NSArray *kvsV) {
    NSMutableString *valueString = [[NSMutableString alloc] init];
    int             count = [kvsN count];

    for (int i = 0; i < count; i++) {
        NSString *name = kvsN[i];
        double   value = [kvsV[i] doubleValue];

        // PMU tdev sensors (e.g., "PMU tdev1") use sp78 format and need /256 conversion
        // Check for "tdev" followed by a digit to avoid false matches
        NSRange range = [name rangeOfString:@"tdev"];
        if (range.location != NSNotFound) {
            // Verify the pattern is "tdev" followed by a number (e.g., "tdev1", "PMU tdev2")
            if (range.location + 4 < [name length]) {
                unichar nextChar = [name characterAtIndex:range.location + 4];
                if (nextChar >= '1' && nextChar <= '9') {
                    value = value / 256.0;
                }
            }
        }

        NSString *output = [NSString stringWithFormat:@"%s:%lf\n", [name UTF8String], value];
        [valueString appendString:output];
    }

    return valueString;
}

char *getCurrents() {
    NSDictionary    *currentSensors = matching(0xff08, 2);
    NSArray         *currentNames = getProductNames(currentSensors);
    NSArray         *currentValues = getPowerValues(currentSensors);
    NSString        *result = dumpNamesValues(currentNames, currentValues);
    char            *finalStr = strdup([result UTF8String]);

    CFRelease(currentSensors);
    CFRelease(currentNames);
    CFRelease(currentValues);
    CFRelease(result);

    return finalStr;
}

char *getVoltages() {
    NSDictionary    *voltageSensors = matching(0xff08, 3);
    NSArray         *voltageNames = getProductNames(voltageSensors);
    NSArray         *voltageValues = getPowerValues(voltageSensors);
    NSString        *result = dumpNamesValues(voltageNames, voltageValues);
    char            *finalStr = strdup([result UTF8String]);

    CFRelease(voltageSensors);
    CFRelease(voltageNames);
    CFRelease(voltageValues);
    CFRelease(result);

    return finalStr;
}

char *getThermals() {
    NSDictionary    *thermalSensors = matching(0xff00, 5);
    NSArray         *thermalNames = getProductNames(thermalSensors);
    NSArray         *thermalValues = getThermalValues(thermalSensors);
    NSString        *result = dumpThermalNamesValues(thermalNames, thermalValues);
    char            *finalStr = strdup([result UTF8String]);

    CFRelease(thermalSensors);
    CFRelease(thermalNames);
    CFRelease(thermalValues);
    CFRelease(result);

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

// GetCurrent returns detected HID current sensor results.
func GetCurrent() map[string]any {
	cStr := C.getCurrents()
	defer C.free(unsafe.Pointer(cStr)) //nolint:wsl,nlreturn

	return getGeneric("A", cStr)
}

// GetVoltage returns detected HID voltage sensor results.
func GetVoltage() map[string]any {
	cStr := C.getVoltages()
	defer C.free(unsafe.Pointer(cStr)) //nolint:wsl,nlreturn

	return getGeneric("V", cStr)
}

// GetTemp returns detected HID temperature sensor results.
func GetTemperature() map[string]any {
	cStr := C.getThermals()
	defer C.free(unsafe.Pointer(cStr)) //nolint:wsl,nlreturn

	return getGeneric("°C", cStr)
}
