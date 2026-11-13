// SPDX-FileCopyrightText: Copyright (C) 2026  Dinko Korunic
// SPDX-License-Identifier: GPL-3.0-only

//go:build darwin

package platform

/*
#include <stdlib.h>
#include <sys/sysctl.h>

static int32_t sysctl_int32(const char *name) {
    int32_t val = 0;
    size_t size = sizeof(val);
    sysctlbyname(name, &val, &size, NULL, 0);
    return val;
}
*/
import "C"

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"unsafe"
)

// PerfLevel describes one CPU performance tier as reported by the macOS sysctl
// hw.perflevel{N}.* hierarchy.
type PerfLevel struct {
	Name        string
	PhysicalCPU int
	LogicalCPU  int
}

// sysctlString returns the string value of the named sysctl, or "" on failure.
func sysctlString(key string) string {
	name := C.CString(key)
	defer C.free(unsafe.Pointer(name))

	var size C.size_t

	if ret := C.sysctlbyname(name, nil, &size, nil, 0); ret < 0 || size == 0 {
		return ""
	}

	buf := C.malloc(size)
	if buf == nil {
		return ""
	}
	defer C.free(buf)

	if ret := C.sysctlbyname(name, buf, &size, nil, 0); ret < 0 {
		return ""
	}

	return C.GoString((*C.char)(buf))
}

// getModel returns the hardware model identifier (e.g. "Mac16,1") via the hw.model sysctl,
// or an empty string if the sysctl call fails. The result is cached after the first call.
var getModel = sync.OnceValue(func() string { return sysctlString("hw.model") })

// getProduct looks the model up in products and, for model IDs newer than the
// table, falls back to the machdep.cpu.brand_string sysctl. Cached.
var getProduct = sync.OnceValues(func() (Product, bool) {
	if p, ok := products[getModel()]; ok {
		return p, true
	}

	return productFromBrand(sysctlString("machdep.cpu.brand_string"))
})

// productFromBrand derives Family and CPU from an Apple Silicon brand string
// ("Apple M3 Max" → "M3", "M3 Max"). Intel brand strings return false so callers
// keep their GOARCH fallback.
func productFromBrand(brand string) (Product, bool) {
	cpu, ok := strings.CutPrefix(brand, "Apple ")
	family, _, _ := strings.Cut(cpu, " ")

	if !ok || len(family) < 2 || family[1] < '0' || family[1] > '9' {
		return Product{}, false
	}

	return Product{Name: "Mac (" + cpu + ")", Family: family, CPU: cpu, Year: 0}, true
}

// GetFamily returns the CPU platform family name (e.g. "M4", "Intel") for the current hardware,
// or "Unknown" when neither the model identifier nor the CPU brand string identifies it.
func GetFamily() string {
	p, ok := getProduct()
	if !ok {
		return "Unknown"
	}

	return p.Family
}

// GetModelID returns the raw hardware model identifier (e.g. "Mac16,1") as reported by the
// hw.model sysctl, or an empty string if the sysctl call fails.
func GetModelID() string {
	return getModel()
}

// GetProduct returns the full Product metadata for the current hardware.
// The boolean reports whether the model was found in the products map or
// derived from the CPU brand string.
func GetProduct() (Product, bool) {
	return getProduct()
}

// GetSKULayout returns the validated core composition for the current machine's
// SKU (looked up by Product.CPU). Returns the zero SKULayout and false when the
// model is unknown or the SKU has no roster entry — callers should treat that
// as "no validation possible" rather than as an error.
func GetSKULayout() (SKULayout, bool) {
	p, ok := getProduct()
	if !ok {
		return SKULayout{}, false
	}

	layout, ok := skuLayouts[p.CPU]

	return layout, ok
}

// LookupSKULayout returns the roster entry for the given Product.CPU string.
// Exposed for testing and for callers that already hold a Product struct.
func LookupSKULayout(cpu string) (SKULayout, bool) {
	layout, ok := skuLayouts[cpu]

	return layout, ok
}

// SKUs returns every Product.CPU that has a roster entry, in no particular
// order. Exposed so tests can derive the set of valid platform tags.
func SKUs() []string {
	return slices.Collect(maps.Keys(skuLayouts))
}

// GetTotalCPU returns the total physical and logical CPU counts via the
// hw.physicalcpu and hw.logicalcpu sysctls.
func GetTotalCPU() (int, int) {
	pcpuKey := C.CString("hw.physicalcpu")
	defer C.free(unsafe.Pointer(pcpuKey))

	lcpuKey := C.CString("hw.logicalcpu")
	defer C.free(unsafe.Pointer(lcpuKey))

	physical := int(C.sysctl_int32(pcpuKey))
	logical := int(C.sysctl_int32(lcpuKey))

	return physical, logical
}

// GetPerfLevels returns the CPU performance levels for the current machine,
// ordered from highest to lowest performance (perflevel0 first).
// Returns nil if hw.nperflevels is unavailable or zero.
func GetPerfLevels() []PerfLevel {
	nKey := C.CString("hw.nperflevels")
	n := int(C.sysctl_int32(nKey))
	C.free(unsafe.Pointer(nKey))

	if n <= 0 {
		return nil
	}

	levels := make([]PerfLevel, 0, n)

	for i := range n {
		levelName := sysctlString(fmt.Sprintf("hw.perflevel%d.name", i))
		if levelName == "" {
			continue
		}

		pcpuKey := C.CString(fmt.Sprintf("hw.perflevel%d.physicalcpu", i))
		lcpuKey := C.CString(fmt.Sprintf("hw.perflevel%d.logicalcpu", i))

		pcpu := int(C.sysctl_int32(pcpuKey))
		lcpu := int(C.sysctl_int32(lcpuKey))

		C.free(unsafe.Pointer(pcpuKey))
		C.free(unsafe.Pointer(lcpuKey))

		levels = append(levels, PerfLevel{
			Name:        levelName,
			PhysicalCPU: pcpu,
			LogicalCPU:  lcpu,
		})
	}

	if len(levels) == 0 {
		return nil
	}

	return levels
}
