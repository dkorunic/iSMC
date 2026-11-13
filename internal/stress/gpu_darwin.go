// SPDX-FileCopyrightText: Copyright (C) 2026  Dinko Korunic
// SPDX-License-Identifier: GPL-3.0-only

//go:build darwin

package stress

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Foundation -framework Metal
#import <Foundation/Foundation.h>
#import <Metal/Metal.h>

// Compute-bound on purpose: each thread runs a long FMA chain in registers and
// writes a single float, so the load lands on the GPU ALUs instead of memory
// bandwidth (which would also heat the Tm* memory sensors and blur the phase).
static const char *gpuKernelSrc =
    "#include <metal_stdlib>\n"
    "using namespace metal;\n"
    "kernel void burn(device float *out [[buffer(0)]],\n"
    "                 uint id [[thread_position_in_grid]]) {\n"
    "    float a = float(id) * 1e-7f + 1.0001f, b = 1.0003f, c = 1.0007f, d = 1.0013f;\n"
    "    for (uint i = 0; i < 8192; i++) {\n"
    "        a = fma(a, 0.99991f, 0.00007f);\n"
    "        b = fma(b, 0.99993f, 0.00005f);\n"
    "        c = fma(c, 0.99995f, 0.00003f);\n"
    "        d = fma(d, 0.99997f, 0.00001f);\n"
    "    }\n"
    "    out[id] = a + b + c + d;\n"
    "}\n";

enum { gpuThreads = 1 << 20, gpuGroup = 256 };

// Created once and kept for the process lifetime (never released).
static id<MTLCommandQueue> gpuQueue;
static id<MTLComputePipelineState> gpuPipeline;
static id<MTLBuffer> gpuOut;

static int gpu_init(void) {
    if (gpuPipeline != nil) return 0;
    @autoreleasepool {
        id<MTLDevice> dev = MTLCreateSystemDefaultDevice();
        if (dev == nil) return 1;
        NSError *err = nil;
        id<MTLLibrary> lib = [dev newLibraryWithSource:[NSString stringWithUTF8String:gpuKernelSrc]
                                               options:nil error:&err];
        if (lib == nil) return 2;
        id<MTLFunction> fn = [lib newFunctionWithName:@"burn"];
        gpuPipeline = [dev newComputePipelineStateWithFunction:fn error:&err];
        if (gpuPipeline == nil) return 3;
        gpuQueue = [dev newCommandQueue];
        gpuOut = [dev newBufferWithLength:gpuThreads * sizeof(float)
                                  options:MTLResourceStorageModePrivate];
        if (gpuQueue == nil || gpuOut == nil) return 4;
    }
    return 0;
}

// gpu_dispatch runs one full-grid dispatch and blocks until it completes.
static void gpu_dispatch(void) {
    @autoreleasepool {
        id<MTLCommandBuffer> cb = [gpuQueue commandBuffer];
        id<MTLComputeCommandEncoder> enc = [cb computeCommandEncoder];
        [enc setComputePipelineState:gpuPipeline];
        [enc setBuffer:gpuOut offset:0 atIndex:0];
        [enc dispatchThreadgroups:MTLSizeMake(gpuThreads / gpuGroup, 1, 1)
            threadsPerThreadgroup:MTLSizeMake(gpuGroup, 1, 1)];
        [enc endEncoding];
        [cb commit];
        [cb waitUntilCompleted];
    }
}
*/
import "C"

import (
	"errors"
	"fmt"
	"runtime"
)

// ErrGPUUnavailable is returned by StartGPU when no Metal device or pipeline is available.
var ErrGPUUnavailable = errors.New("metal GPU load unavailable")

// StartGPU keeps the default Metal GPU busy with a compute-bound kernel until
// the returned stop function is called; stop blocks until the in-flight
// dispatch finishes. It errors when no Metal device or pipeline is available.
func StartGPU() (func(), error) {
	if rc := C.gpu_init(); rc != 0 {
		return nil, fmt.Errorf("%w (init step %d)", ErrGPUUnavailable, int(rc))
	}

	done := make(chan struct{})
	exited := make(chan struct{})

	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(exited)

		for {
			select {
			case <-done:
				return
			default:
				C.gpu_dispatch()
			}
		}
	}()

	return func() {
		close(done)
		<-exited
	}, nil
}
