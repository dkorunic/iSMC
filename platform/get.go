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
*/
import "C"

import (
	"unsafe"
)

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
