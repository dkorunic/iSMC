package gosmc

// IOReturn values
const (
	IOReturnSuccess         = 0x0   // OK
	IOReturnError           = 0x2bc // general error
	IOReturnNoMemory        = 0x2bd // can't allocate memory
	IOReturnNoResources     = 0x2be // resource shortage
	IOReturnIPCError        = 0x2bf // error during IPC
	IOReturnNoDevice        = 0x2c0 // no such device
	IOReturnNotPrivileged   = 0x2c1 // privilege violation
	IOReturnBadArgument     = 0x2c2 // invalid argument
	IOReturnLockedRead      = 0x2c3 // device read locked
	IOReturnLockedWrite     = 0x2c4 // device write locked
	IOReturnExclusiveAccess = 0x2c5 // exclusive access and
	//   device already open
	IOReturnBadMessageID = 0x2c6 // sent/received messages
	//   had different msg_id
	IOReturnUnsupported   = 0x2c7 // unsupported function
	IOReturnVMError       = 0x2c8 // misc. VM failure
	IOReturnInternalError = 0x2c9 // internal error
	IOReturnIOError       = 0x2ca // General I/O error

	IOReturnCannotLock  = 0x2cc // can't acquire lock
	IOReturnNotOpen     = 0x2cd // device not open
	IOReturnNotReadable = 0x2ce // read not supported
	IOReturnNotWritable = 0x2cf // write not supported
	IOReturnNotAligned  = 0x2d0 // alignment error
	IOReturnBadMedia    = 0x2d1 // Media Error
	IOReturnStillOpen   = 0x2d2 // device(s) still open
	IOReturnRLDError    = 0x2d3 // rld failure
	IOReturnDMAError    = 0x2d4 // DMA failure
	IOReturnBusy        = 0x2d5 // Device Busy
	IOReturnTimeout     = 0x2d6 // I/O Timeout
	IOReturnOffline     = 0x2d7 // device offline
	IOReturnNotReady    = 0x2d8 // not ready
	IOReturnNotAttached = 0x2d9 // device not attached
	IOReturnNoChannels  = 0x2da // no DMA channels left
	IOReturnNoSpace     = 0x2db // no space for data

	IOReturnPortExists = 0x2dd // port already exists
	IOReturnCannotWire = 0x2de // can't wire down
	//   physical memory
	IOReturnNoInterrupt     = 0x2df // no interrupt attached
	IOReturnNoFrames        = 0x2e0 // no DMA frames enqueued
	IOReturnMessageTooLarge = 0x2e1 // oversized msg received
	//   on interrupt port
	IOReturnNotPermitted     = 0x2e2 // not permitted
	IOReturnNoPower          = 0x2e3 // no power to device
	IOReturnNoMedia          = 0x2e4 // media not present
	IOReturnUnformattedMedia = 0x2e5 // media not formatted
	IOReturnUnsupportedMode  = 0x2e6 // no such mode
	IOReturnUnderrun         = 0x2e7 // data underrun
	IOReturnOverrun          = 0x2e8 // data overrun
	IOReturnDeviceError      = 0x2e9 // the device is not working properly!
	IOReturnNoCompletion     = 0x2ea // a completion routine is required
	IOReturnAborted          = 0x2eb // operation aborted
	IOReturnNoBandwidth      = 0x2ec // bus bandwidth would be exceeded
	IOReturnNotResponding    = 0x2ed // device not responding
	IOReturnIsoTooOld        = 0x2ee // isochronous I/O request for distant past!
	IOReturnIsoTooNew        = 0x2ef // isochronous I/O request for distant future
	IOReturnNotFound         = 0x2f0 // data was not found
	IOReturnInvalid          = 0x1   // should never be seen
)

// OP Values
const (
	OPNone = iota
	OPList
	OPRead
	OPReadFan
	OPWrite
	OPBruteForce
)

// Kernel values
const (
	KernelIndexSMC = 2
)

// SMC CMD values
const (
	CMDReadBytes   = 5
	CMDWriteBytes  = 6
	CMDReadIndex   = 8
	CMDReadKeyinfo = 9
	CMDReadPlimit  = 11
	CMDReadVers    = 12
)

// SMC Type values
const (
	TypeFPE2 = "fpe2"
	TypeFP2E = "fp2e"
	TypeFP4C = "fp4c"
	TypeCH8  = "ch8*"
	TypeSP78 = "sp78"
	TypeSP4B = "sp4b"
	TypeFP5B = "fp5b"
	TypeFP88 = "fp88"
	TypeUI8  = "ui8"
	TypeUI16 = "ui16"
	TypeUI32 = "ui32"
	TypeSI8  = "si8"
	TypeSI16 = "si16"
	TypeSI32 = "si32"
	TypeFLAG = "flag"
	TypeFDS  = "{fds"
	TypeFLT  = "flt"
)

// SMC Size values
const (
	TypeFPXXSize = 2
	TypeSPXXSize = 2
	TypeUI8Size  = 1
	TypeUI16Size = 2
	TypeUI32Size = 4
	TypeSI8Size  = 1
	TypeSI16Size = 2
	TypeSI32Size = 4
	TypeFLAGSize = 1
)
