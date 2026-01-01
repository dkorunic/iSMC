//go:build darwin
// +build darwin

package gosmc

// #cgo CFLAGS: -O2 -Wall
// #cgo LDFLAGS: -framework IOKit
// #include <stdlib.h>
// #include "smc.h"
import "C"

import (
	"unsafe"
)

// DataVers is IOKit DataVers struct
type DataVers struct {
	Major    uint8
	Minor    uint8
	Build    uint8
	Reserved [1]uint8
	Release  uint16
}

// PLimitData is IOKit PLimitData struct
type PLimitData struct {
	Version   uint16
	Length    uint16
	CPUPLimit uint32
	GPUPLimit uint32
	MemPLimit uint32
}

// KeyInfo is IOKit KeyInfo struct
type KeyInfo struct {
	DataSize       uint32
	DataType       uint32
	DataAttributes uint8
}

// SMCKeyData is IOKit SMCKeyData struct
type SMCKeyData struct {
	Key        uint32
	Vers       DataVers
	PLimitData PLimitData
	KeyInfo    KeyInfo
	Result     uint8
	Status     uint8
	Data8      uint8
	Data32     uint32
	Bytes      SMCBytes
}

// SMCVal is IOKit SMCVal struct
type SMCVal struct {
	Key      UInt32Char
	DataSize uint32
	DataType UInt32Char
	Bytes    SMCBytes
}

// UInt32Char is IOKit UInt32Char type
type UInt32Char [5]byte

func (bs UInt32Char) toC() C.UInt32Char_t {
	var xs C.UInt32Char_t
	for i := range bs {
		xs[i] = C.char(bs[i])
	}
	return xs
}

// ToString return as string
func (bs UInt32Char) ToString() string {
	return string(bs[:])
}

func uint32CharFromC(xs C.UInt32Char_t) UInt32Char {
	var bs UInt32Char
	for i := range xs {
		bs[i] = byte(xs[i])
	}
	return bs
}

// SMCBytes is IOKit UInt32Char SMCBytes
type SMCBytes [32]byte

func (bs SMCBytes) toC() [32]C.uchar {
	var xs [32]C.uchar
	for i := range bs {
		xs[i] = C.uchar(bs[i])
	}
	return xs
}

func smcBytesFromC(xs C.SMCBytes_t) SMCBytes {
	var bs SMCBytes
	for i := range xs {
		bs[i] = byte(xs[i])
	}
	return bs
}

// SMCOpen wrapper for Apple IOKit SMCOpen
func SMCOpen(service string) (connection uint, result int) {
	svc := C.CString(service)
	defer C.free(unsafe.Pointer(svc))

	var conn C.uint
	result = int(C.SMCOpen(svc, &conn))
	connection = uint(conn)

	return connection, result
}

// SMCClose wrapper for Apple IOKit SMCClose
func SMCClose(connection uint) int {
	return int(C.SMCClose(C.uint(connection)))
}

// SMCReadKey wrapper for Apple IOKit SMCReadKey
func SMCReadKey(connection uint, key string) (*SMCVal, int) {
	k := C.CString(key)
	defer C.free(unsafe.Pointer(k))

	v := C.SMCVal_t{}
	result := C.SMCReadKey(C.uint(connection), k, &v)

	return &SMCVal{
		Key:      uint32CharFromC(v.key),
		DataSize: uint32(v.dataSize),
		DataType: uint32CharFromC(v.dataType),
		Bytes:    smcBytesFromC(v.bytes),
	}, int(result)
}

// SMCCall wrapper for Apple IOKit SMCCall
func SMCCall(connection uint, index int, inputStruct *SMCKeyData) (*SMCKeyData, int) {
	in := C.SMCKeyData_t{
		key: C.uint(inputStruct.Key),
		vers: C.SMCKeyData_vers_t{
			major:    C.uchar(inputStruct.Vers.Major),
			minor:    C.uchar(inputStruct.Vers.Minor),
			build:    C.uchar(inputStruct.Vers.Build),
			reserved: [1]C.uchar{C.uchar(inputStruct.Vers.Reserved[0])},
			release:  C.ushort(inputStruct.Vers.Release),
		},
		pLimitData: C.SMCKeyData_pLimitData_t{
			version:   C.ushort(inputStruct.PLimitData.Version),
			length:    C.ushort(inputStruct.PLimitData.Length),
			cpuPLimit: C.uint(inputStruct.PLimitData.CPUPLimit),
			gpuPLimit: C.uint(inputStruct.PLimitData.GPUPLimit),
			memPLimit: C.uint(inputStruct.PLimitData.MemPLimit),
		},
		keyInfo: C.SMCKeyData_keyInfo_t{
			dataSize:       C.uint(inputStruct.KeyInfo.DataSize),
			dataType:       C.uint(inputStruct.KeyInfo.DataType),
			dataAttributes: C.uchar(inputStruct.KeyInfo.DataAttributes),
		},
		result: C.uchar(inputStruct.Result),
		status: C.uchar(inputStruct.Status),
		data8:  C.uchar(inputStruct.Data8),
		data32: C.uint(inputStruct.Data32),
		bytes:  inputStruct.Bytes.toC(),
	}

	out := C.SMCKeyData_t{}
	result := C.SMCCall(C.uint(connection), C.int(index), &in, &out)

	return &SMCKeyData{
		Key: uint32(out.key),
		Vers: DataVers{
			Major:    uint8(out.vers.major),
			Minor:    uint8(out.vers.minor),
			Build:    uint8(out.vers.build),
			Reserved: [1]uint8{uint8(out.vers.reserved[0])},
			Release:  uint16(out.vers.release),
		},
		PLimitData: PLimitData{
			Version:   uint16(out.pLimitData.version),
			Length:    uint16(out.pLimitData.length),
			CPUPLimit: uint32(out.pLimitData.cpuPLimit),
			GPUPLimit: uint32(out.pLimitData.gpuPLimit),
			MemPLimit: uint32(out.pLimitData.memPLimit),
		},
		KeyInfo: KeyInfo{
			DataSize:       uint32(out.keyInfo.dataSize),
			DataType:       uint32(out.keyInfo.dataType),
			DataAttributes: uint8(out.keyInfo.dataAttributes),
		},
		Result: uint8(out.result),
		Status: uint8(out.status),
		Data8:  uint8(out.data8),
		Data32: uint32(out.data32),
		Bytes:  smcBytesFromC(out.bytes),
	}, int(result)
}

// SMCWriteKey wrapper for Apple IOKit SMCWriteKey
func SMCWriteKey(connection uint, val *SMCVal) int {
	result := C.SMCWriteKey(C.uint(connection), &C.SMCVal_t{
		key:      val.Key.toC(),
		dataSize: C.uint(val.DataSize),
		dataType: val.DataType.toC(),
		bytes:    val.Bytes.toC(),
	})
	return int(result)
}

// SMCWriteKeyUnsafe wrapper for Apple IOKit SMCWriteKeyUnsafe
func SMCWriteKeyUnsafe(connection uint, val *SMCVal) int {
	result := C.SMCWriteKey(C.uint(connection), &C.SMCVal_t{
		key:      val.Key.toC(),
		dataSize: C.uint(val.DataSize),
		dataType: val.DataType.toC(),
		bytes:    val.Bytes.toC(),
	})
	return int(result)
}
