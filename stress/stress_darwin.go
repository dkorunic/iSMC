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

// Package stress provides CPU affinity helpers for macOS.
package stress

/*
#include <mach/mach.h>
#include <mach/thread_act.h>
#include <mach/thread_policy.h>
#include <pthread.h>

static int set_affinity_tag(int tag) {
    thread_affinity_policy_data_t policy;
    policy.affinity_tag = tag;
    return (int)thread_policy_set(
        mach_thread_self(),
        THREAD_AFFINITY_POLICY,
        (thread_policy_t)&policy,
        THREAD_AFFINITY_POLICY_COUNT
    );
}

static int set_qos_class(int cls) {
    return pthread_set_qos_class_self_np((qos_class_t)cls, 0);
}
*/
import "C"

const (
	// QoSUserInitiated maps to QOS_CLASS_USER_INITIATED (0x19).
	// The OS scheduler prefers to run threads with this class on P-cores (performance cores).
	QoSUserInitiated = 0x19

	// QoSBackground maps to QOS_CLASS_BACKGROUND (0x09).
	// The OS scheduler prefers to run threads with this class on E-cores (efficiency cores).
	QoSBackground = 0x09
)

// SetAffinityTag sets the macOS THREAD_AFFINITY_POLICY tag for the calling OS thread.
//
// Threads sharing the same non-zero tag are preferentially scheduled together on the
// same hardware thread; threads with different non-zero tags are preferentially scheduled
// on different hardware threads. Tag 0 clears any previous affinity preference.
//
// This is a hint only – the scheduler may not honour it. The function must be called from
// a goroutine that has already called runtime.LockOSThread so that the tag is bound to a
// stable OS thread for the lifetime of the stress run.
//
// Returns the raw kern_return_t value (0 = KERN_SUCCESS).
func SetAffinityTag(tag int) int {
	return int(C.set_affinity_tag(C.int(tag)))
}

// SetQoS sets the QoS class for the calling OS thread via pthread_set_qos_class_self_np.
//
// Use QoSUserInitiated to bias scheduling toward P-cores and QoSBackground to bias
// toward E-cores. This is a hint only; the function must be called from a goroutine
// that has already called runtime.LockOSThread.
//
// Returns 0 on success, a non-zero errno value on failure.
func SetQoS(qosClass int) int {
	return int(C.set_qos_class(C.int(qosClass)))
}
