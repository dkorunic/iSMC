// SPDX-FileCopyrightText: Copyright (C) 2026  Dinko Korunic
// SPDX-License-Identifier: GPL-3.0-only

//go:build darwin

package stress

/*
#include <mach/mach.h>

// cpu_ticks writes busy (user+system+nice) and total tick counters for up to
// max logical CPUs into out[2*i], out[2*i+1] and returns the CPU count written,
// or -1 when host_processor_info fails.
static int cpu_ticks(unsigned int *out, int max) {
    natural_t n = 0;
    processor_info_array_t info;
    mach_msg_type_number_t cnt = 0;

    if (host_processor_info(mach_host_self(), PROCESSOR_CPU_LOAD_INFO, &n, &info, &cnt) != KERN_SUCCESS)
        return -1;

    processor_cpu_load_info_t l = (processor_cpu_load_info_t)info;
    int m = (int)n < max ? (int)n : max;

    for (int i = 0; i < m; i++) {
        unsigned int busy = l[i].cpu_ticks[CPU_STATE_USER] + l[i].cpu_ticks[CPU_STATE_SYSTEM] +
                            l[i].cpu_ticks[CPU_STATE_NICE];
        out[2 * i] = busy;
        out[2 * i + 1] = busy + l[i].cpu_ticks[CPU_STATE_IDLE];
    }

    vm_deallocate(mach_task_self(), (vm_address_t)info, cnt * sizeof(integer_t));

    return m;
}
*/
import "C"

import "unsafe"

// maxCPUs bounds the tick buffer; the largest Apple Silicon part has 32 CPUs.
const maxCPUs = 256

// CPUTicks is one logical CPU's cumulative busy and total scheduler ticks.
// The counters are 32-bit and wrap; BusyShare subtracts them modulo 2^32.
type CPUTicks struct {
	Busy, Total uint32
}

// ReadCPUTicks returns cumulative tick counters per logical CPU, indexed by the
// kernel's CPU number, or nil when host_processor_info fails.
func ReadCPUTicks() []CPUTicks {
	buf := make([]C.uint, 2*maxCPUs)

	n := int(C.cpu_ticks((*C.uint)(unsafe.Pointer(&buf[0])), maxCPUs))
	if n <= 0 {
		return nil
	}

	out := make([]CPUTicks, n)
	for i := range out {
		out[i] = CPUTicks{Busy: uint32(buf[2*i]), Total: uint32(buf[2*i+1])}
	}

	return out
}

// BusyShare returns each CPU's busy fraction (0–1) between two ReadCPUTicks
// snapshots, or nil when they are missing or differ in CPU count.
func BusyShare(before, after []CPUTicks) []float32 {
	if len(before) == 0 || len(before) != len(after) {
		return nil
	}

	out := make([]float32, len(before))

	for i := range before {
		// uint32 subtraction is wrap-safe.
		if total := after[i].Total - before[i].Total; total > 0 {
			out[i] = float32(after[i].Busy-before[i].Busy) / float32(total)
		}
	}

	return out
}
