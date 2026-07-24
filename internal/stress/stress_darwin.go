// SPDX-FileCopyrightText: Copyright (C) 2026  Dinko Korunic
// SPDX-License-Identifier: GPL-3.0-only

//go:build darwin

// Package stress provides CPU affinity helpers for macOS.
package stress

/*
#include <pthread.h>

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
