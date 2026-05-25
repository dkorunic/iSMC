// SPDX-FileCopyrightText: Copyright (C) 2026  Dinko Korunic
// SPDX-License-Identifier: GPL-3.0-only

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
	QoSUserInteractive = 0x21 // QOS_CLASS_USER_INTERACTIVE; biases toward Super (M5+) or P-cores.
	QoSUserInitiated   = 0x19 // QOS_CLASS_USER_INITIATED; biases toward P-cores.
	QoSBackground      = 0x09 // QOS_CLASS_BACKGROUND; biases toward E-cores.
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
