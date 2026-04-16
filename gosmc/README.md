# gosmc

A Go package providing CGo bindings to the Apple System Management Controller (SMC) via macOS IOKit. It wraps the C-level SMC interface — open/close, key-info lookup, key read, and key write — into idiomatic Go types.

**macOS only.** All source files carry `//go:build darwin`. The package is a nested Go module (`github.com/dkorunic/iSMC/gosmc`) and requires `CGO_ENABLED=1`.

## API

### Connection management

```go
// Open a connection to the named IOKit service (typically "AppleSMC").
// Returns (connection handle, IOReturn result code).
connection, result := gosmc.SMCOpen("AppleSMC")
if result != gosmc.IOReturnSuccess {
    // handle error
}
defer gosmc.SMCClose(connection)
```

### Reading a key

```go
val, result := gosmc.SMCReadKey(connection, "TC0P")
if result != gosmc.IOReturnSuccess {
    // handle error
}
fmt.Printf("key=%s type=%s size=%d bytes=%v\n",
    val.Key.ToString(), val.DataType.ToString(), val.DataSize, val.Bytes[:val.DataSize])
```

`SMCReadKey` internally uses a process-wide sorted-array cache (up to 2048 entries, binary-searched) so that key-info lookups hit IOKit only on first access. Subsequent reads for the same key avoid the blocking `SMCGetKeyInfo` round-trip entirely. Keys confirmed to be absent or restricted are negatively cached, so repeated lookups for unavailable keys also skip IOKit.

### Low-level call

```go
input := &gosmc.SMCKeyData{
    Data8:  gosmc.CMDReadIndex,
    Data32: index,
}
output, result := gosmc.SMCCall(connection, gosmc.KernelIndexSMC, input)
```

`SMCCall` maps directly to `IOConnectCallStructMethod`. After a successful transport call it validates that the driver wrote enough bytes to cover at least the SMC result field; if the response is truncated it returns `kIOReturnUnderrun`. Use it when you need raw index-based enumeration (e.g. iterating all keys via `CMDReadIndex`).

### Writing a key

```go
val := &gosmc.SMCVal{DataSize: 1}
val.Key[0], val.Key[1], val.Key[2], val.Key[3] = 'F', '0', 'T', 'g'
val.DataType[0], val.DataType[1], val.DataType[2], val.DataType[3] = 'f', 'p', '2', 'e'
val.Bytes[0], val.Bytes[1] = 0x0C, 0x00

result := gosmc.SMCWriteKey(connection, val)       // validates DataSize against current value
result  = gosmc.SMCWriteKeyUnsafe(connection, val)  // skips the pre-read size check
```

`SMCWriteKey` performs a pre-read to verify that `val.DataSize` matches the key's current size before writing. `SMCWriteKeyUnsafe` skips that check.

## Types

| Go type | Description |
|---------|-------------|
| `SMCVal` | Key name, data type, data size, and raw bytes for one SMC key |
| `SMCKeyData` | Full IOKit `SMCKeyData_t` struct used by `SMCCall` |
| `SMCBytes` | `[32]byte` — capped at the kernel's `SMC_MAX_DATA_SIZE` per-read limit |
| `UInt32Char` | `[5]byte` — null-terminated 4-character ASCII key or type name |
| `KeyInfo` | Data size, data type, and attributes for one key (cache entry value) |
| `DataVers` | SMC firmware version struct |
| `PLimitData` | SMC power-limit struct |

## Constants

### IOReturn result codes (`values.go`)

```go
gosmc.IOReturnSuccess   // 0x000 — operation succeeded
gosmc.IOReturnError     // 0x2bc — general error
gosmc.IOReturnNoDevice  // 0x2c0 — SMC not found
gosmc.IOReturnBusy      // 0x2d5 — device busy
gosmc.IOReturnTimeout   // 0x2d6 — I/O timeout
// … full list in values.go
```

### SMC command codes

```go
gosmc.CMDReadBytes    // 5  — read key value bytes
gosmc.CMDWriteBytes   // 6  — write key value bytes
gosmc.CMDReadIndex    // 8  — read key at numeric index
gosmc.CMDReadKeyinfo  // 9  — read key metadata
gosmc.CMDReadPlimit   // 11 — read power-limit data
gosmc.CMDReadVers     // 12 — read SMC firmware version
```

### SMC data types

```go
gosmc.TypeSP78  // "sp78" — signed 7.8 fixed-point (most temperature sensors)
gosmc.TypeFLT   // "flt"  — 32-bit IEEE 754 float
gosmc.TypeUI8   // "ui8"  — unsigned 8-bit integer
gosmc.TypeUI16  // "ui16" — unsigned 16-bit integer
gosmc.TypeUI32  // "ui32" — unsigned 32-bit integer
gosmc.TypeFLAG  // "flag" — single boolean byte
// … full list in values.go
```

## Internal implementation notes

- **`smc.c`** — the C layer. Key functions: `SMCOpen`, `SMCClose`, `SMCCall`, `SMCReadKey`, `SMCWriteKey`, `SMCGetKeyInfo`. Internal static helpers `smcPackKeyBytes` (4-char key string → big-endian `UInt32`) and `smcUnpackKeyBytes` (`UInt32` → 4-char null-terminated string) are not part of the public API. `SMCOpen` returns `kIOReturnNoMemory` if `IOServiceMatching` fails before any IOKit call is made.
- **Key-info cache** — `SMCGetKeyInfo` maintains a process-global sorted array of up to 2048 `SMCKeyInfoCacheEntry_t` structs. Each entry carries a `negative` flag: positive entries store the key's metadata; negative entries record that the key is known to be absent or restricted, preventing repeated IOKit round-trips for unavailable keys. Lookups use binary search (O(log N)) and return +1 (hit), -1 (negative hit — skip IOKit), or 0 (miss). Inserts use binary search to find the insertion point and `memmove` to shift existing entries, keeping the array sorted at all times. A full cache blocks new insertions but never blocks in-place upgrades of negative entries to positive — the upgrade check runs before the capacity guard. `SMCClose` acquires the lock and flushes the cache before closing the port, preventing Mach port number recycling from causing a new connection to receive stale cached key-info from the old one. The `os_unfair_lock` is held only for the short cache-lookup and cache-insert sections; the blocking `SMCCall` for a cache miss runs outside the lock. After re-acquiring the lock to insert a result, the code re-checks that `g_cachedConn == conn` to discard results that belong to a connection that was switched by another thread while `SMCCall` was in progress.
- **`gosmc.go`** — the Go layer. Converts between Go structs and their `C.*` equivalents for each API call.
- **`values.go`** — IOReturn codes, SMC command codes, SMC type name constants, and SMC type size constants.

## Building

```sh
CGO_ENABLED=1 go build ./...
```

The package links against `-framework IOKit` (declared via `#cgo LDFLAGS` in `gosmc.go`).

## References

- [Apple IOKit documentation](https://developer.apple.com/documentation/iokit)
- [OS-X-FakeSMC-kozlek](https://github.com/RehabMan/OS-X-FakeSMC-kozlek) — original C SMC library this package wraps
- [osx-cpu-temp](https://github.com/lavoiesl/osx-cpu-temp) — reference implementation
