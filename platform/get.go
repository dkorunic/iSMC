// Copyright (C) 2026  Dinko Korunic
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
	"unsafe"
)

// PerfLevel describes one CPU performance tier as reported by the macOS sysctl
// hw.perflevel{N}.* hierarchy.
type PerfLevel struct {
	Name        string // hw.perflevelN.name, e.g. "Performance" / "Efficiency"
	PhysicalCPU int    // hw.perflevelN.physicalcpu
	LogicalCPU  int    // hw.perflevelN.logicalcpu
}

// getModel returns the hardware model identifier (e.g. "Mac16,1") via the hw.model sysctl,
// or an empty string if the sysctl call fails.
func getModel() string {
	name := C.CString("hw.model")
	defer C.free(unsafe.Pointer(name))

	var size C.size_t

	if ret := C.sysctlbyname(name, nil, &size, nil, 0); ret < 0 || size == 0 {
		return ""
	}

	buf := C.malloc(size)
	defer C.free(buf)

	C.sysctlbyname(name, buf, &size, nil, 0)

	return C.GoString((*C.char)(buf))
}

// GetFamily returns the CPU platform family name (e.g. "M4", "Intel") for the current hardware,
// or "Unknown" when the model identifier is not in the products map.
func GetFamily() string {
	p, ok := products[getModel()]
	if !ok {
		return "Unknown"
	}

	return p.Family
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
		nameKey := C.CString(fmt.Sprintf("hw.perflevel%d.name", i))
		var size C.size_t

		if ret := C.sysctlbyname(nameKey, nil, &size, nil, 0); ret < 0 || size == 0 {
			C.free(unsafe.Pointer(nameKey))

			continue
		}

		buf := C.malloc(size)
		C.sysctlbyname(nameKey, buf, &size, nil, 0)
		levelName := C.GoString((*C.char)(buf))
		C.free(buf)
		C.free(unsafe.Pointer(nameKey))

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
