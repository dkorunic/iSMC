/*
 * Apple System Management Control (SMC) Tool
 * Copyright (C) 2006 devnull
 * Copyright (C) 2026 Dinko Korunic
 *
 * This program is free software; you can redistribute it and/or
 * modify it under the terms of the GNU General Public License
 * as published by the Free Software Foundation; either version 2
 * of the License, or (at your option) any later version.

 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.

 * You should have received a copy of the GNU General Public License
 * along with this program; if not, write to the Free Software
 * Foundation, Inc., 51 Franklin Street, Fifth Floor, Boston, MA  02110-1301,
 USA.
 */
/*
cc ./smc.c  -o smcutil -framework IOKit -framework CoreFoundation
-Wno-four-char-constants -Wall -g -arch arm64 -arch x86_64
 */

#include <IOKit/IOKitLib.h>
#include <os/lock.h>
#include <stddef.h>
#include <stdio.h>
#include <string.h>

#include "smc.h"

// Cache SMC key info to reduce IOKit round-trips.
// 2048 entries covers all keys on current and near-future Apple Silicon Macs
// (M4/M5 report ~1800 keys).
#define KEY_INFO_CACHE_SIZE 2048

typedef struct {
  UInt32 key;
  int negative; // 1 = negative cache entry (key absent/restricted), 0 = valid
  SMCKeyData_keyInfo_t keyInfo; // only meaningful when negative == 0
} SMCKeyInfoCacheEntry_t;

// All cache globals are static — they are internal implementation details not
// exported to other TUs.
static SMCKeyInfoCacheEntry_t g_keyInfoCache[KEY_INFO_CACHE_SIZE];
static UInt32 g_keyInfoCacheCount = 0;
static io_connect_t g_cachedConn = IO_OBJECT_NULL;
static os_unfair_lock g_keyInfoSpinLock = OS_UNFAIR_LOCK_INIT;

// smcPackKeyBytes packs a 4-character SMC key string into a big-endian UInt32.
// size is clamped to [0, 4] to prevent shift UB (C99 §6.5.7: shifting ≥32 bits
// is undefined).
static UInt32 smcPackKeyBytes(const char *str, int size) {
  UInt32 total = 0;

  if (size < 0)
    size = 0;
  if (size > 4)
    size = 4;

  for (int i = 0; i < size; i++)
    total |= (UInt32)(unsigned char)str[i] << ((size - 1 - i) * 8);

  return total;
}

// smcUnpackKeyBytes converts a big-endian UInt32 key code back to a 4-character
// null-terminated string.
static void smcUnpackKeyBytes(char *str, UInt32 val) {
  snprintf(str, 5, "%c%c%c%c", (val >> 24) & 0xFF, (val >> 16) & 0xFF,
           (val >> 8) & 0xFF, val & 0xFF);
}

kern_return_t SMCOpen(const char *serviceName, io_connect_t *conn) {
  kern_return_t result;
  io_iterator_t iterator;
  io_object_t device;

  CFMutableDictionaryRef matchingDictionary = IOServiceMatching(serviceName);
  if (!matchingDictionary)
    return kIOReturnNoMemory;

  result = IOServiceGetMatchingServices(kIOMainPortDefault, matchingDictionary,
                                        &iterator);
  if (result != kIOReturnSuccess)
    return result;

  device = IOIteratorNext(iterator);
  IOObjectRelease(iterator);
  if (device == IO_OBJECT_NULL)
    return kIOReturnNoDevice;

  result = IOServiceOpen(device, mach_task_self(), 0, conn);
  IOObjectRelease(device);
  if (result != kIOReturnSuccess)
    return result;

  return result;
}

// SMCClose closes the IOKit connection and flushes the key-info cache if it was
// built for this connection, preventing a future SMCOpen from reusing a stale
// cache via Mach port recycling.
kern_return_t SMCClose(io_connect_t conn) {
  os_unfair_lock_lock(&g_keyInfoSpinLock);
  if (g_cachedConn == conn) {
    g_keyInfoCacheCount = 0;
    g_cachedConn = IO_OBJECT_NULL;
  }
  os_unfair_lock_unlock(&g_keyInfoSpinLock);

  return IOServiceClose(conn);
}

kern_return_t SMCCall(io_connect_t conn, UInt32 index,
                      const SMCKeyData_t *inputStructure,
                      SMCKeyData_t *outputStructure) {
  const size_t structureInputSize = sizeof(SMCKeyData_t);
  size_t structureOutputSize = sizeof(SMCKeyData_t);

  kern_return_t result =
      IOConnectCallStructMethod(conn, index, inputStructure, structureInputSize,
                                outputStructure, &structureOutputSize);

  if (result != kIOReturnSuccess)
    return result;

  // Guard against a truncated response: the driver must write at least enough
  // bytes to cover the SMC result code. A short write would leave
  // outputStructure.result at 0 (from the caller's memset), silently masking a
  // bad response from the driver.
  if (structureOutputSize < offsetof(SMCKeyData_t, result) + sizeof(UInt8))
    return kIOReturnUnderrun;

  return kIOReturnSuccess;
}

// keyInfoCacheFind performs a binary search on the always-sorted cache using a
// half-open interval [lo, hi) — safe for UInt32 indices since hi never
// underflows when count is 0. Returns 1 on positive hit (valid keyInfo written
// to *keyInfo when non-NULL),
//        -1 on negative hit (key is known to be absent/restricted — do not call
//        IOKit),
//         0 on miss (key not yet seen).
// Must be called with g_keyInfoSpinLock held.
static int keyInfoCacheFind(UInt32 key, SMCKeyData_keyInfo_t *keyInfo) {
  UInt32 lo = 0, hi = g_keyInfoCacheCount;

  while (lo < hi) {
    UInt32 mid = lo + (hi - lo) / 2;

    if (g_keyInfoCache[mid].key == key) {
      if (g_keyInfoCache[mid].negative)
        return -1;
      if (keyInfo)
        *keyInfo = g_keyInfoCache[mid].keyInfo;
      return 1;
    }

    if (g_keyInfoCache[mid].key < key)
      lo = mid + 1;
    else
      hi = mid;
  }

  return 0;
}

// keyInfoCacheInsert inserts a new entry in sorted order via binary search +
// memmove, keeping the array always sorted so binary-search lookups never need
// a preceding sort step.
//
// Pass keyInfo=NULL to insert a negative cache entry (key known to be absent or
// restricted). If the key already exists:
//   - positive entry: skipped (SMC key metadata is stable at runtime).
//   - negative entry + non-NULL keyInfo: upgraded in place to positive (key
//   became available).
//   - negative entry + NULL keyInfo: skipped (already negatively cached).
//
// When the cache is full, the key falls through to an IOKit round-trip on every
// subsequent lookup. Must be called with g_keyInfoSpinLock held.
static void keyInfoCacheInsert(UInt32 key,
                               const SMCKeyData_keyInfo_t *keyInfo) {
  // Binary search for the insertion point (half-open interval [lo, hi)).
  UInt32 lo = 0, hi = g_keyInfoCacheCount;
  while (lo < hi) {
    UInt32 mid = lo + (hi - lo) / 2;
    if (g_keyInfoCache[mid].key < key)
      lo = mid + 1;
    else
      hi = mid;
  }

  // The binary search lands on the leftmost position where cache[lo].key >=
  // key.
  if (lo < g_keyInfoCacheCount && g_keyInfoCache[lo].key == key) {
    // Key exists in cache. Upgrade a negative entry to positive if we now have
    // valid data. This in-place upgrade does not require a free slot, so it
    // runs regardless of capacity.
    if (keyInfo != NULL && g_keyInfoCache[lo].negative) {
      g_keyInfoCache[lo].keyInfo = *keyInfo;
      g_keyInfoCache[lo].negative = 0;
    }
    return;
  }

  // Only genuinely new entries need a free slot.
  if (g_keyInfoCacheCount >= KEY_INFO_CACHE_SIZE)
    return;

  // Shift elements right to make room, then write at lo.
  memmove(&g_keyInfoCache[lo + 1], &g_keyInfoCache[lo],
          (size_t)(g_keyInfoCacheCount - lo) * sizeof(SMCKeyInfoCacheEntry_t));
  g_keyInfoCache[lo].key = key;
  g_keyInfoCache[lo].negative = (keyInfo == NULL) ? 1 : 0;
  if (keyInfo != NULL)
    g_keyInfoCache[lo].keyInfo = *keyInfo;
  else
    memset(&g_keyInfoCache[lo].keyInfo, 0, sizeof(SMCKeyData_keyInfo_t));
  ++g_keyInfoCacheCount;
}

// SMCGetKeyInfo returns key metadata, using a sorted cache to reduce IOKit
// round-trips. The lock is held only for short cache operations; the blocking
// SMCCall runs lock-free. If conn differs from the connection that populated
// the cache, the cache is flushed to prevent stale key-info from a different
// SMC service being served to the new connection. On re-insertion, the conn is
// re-validated under lock: if another thread switched connections while SMCCall
// was in progress, our result belongs to a different service and is discarded.
// Negative results (key absent or restricted) are cached to avoid repeated
// IOKit round-trips.
static kern_return_t SMCGetKeyInfo(io_connect_t conn, UInt32 key,
                                   SMCKeyData_keyInfo_t *keyInfo) {
  SMCKeyData_t inputStructure;
  SMCKeyData_t outputStructure;
  kern_return_t result;

  // Fast path: binary-search the sorted cache under the lock (no blocking I/O).
  // Flush the cache when a different connection handle is presented.
  os_unfair_lock_lock(&g_keyInfoSpinLock);
  if (g_cachedConn != conn) {
    g_keyInfoCacheCount = 0;
    g_cachedConn = conn;
  }
  int found = keyInfoCacheFind(key, keyInfo);
  os_unfair_lock_unlock(&g_keyInfoSpinLock);

  if (found > 0)
    return kIOReturnSuccess;
  if (found < 0)
    return kIOReturnError; // negative cache: key is known absent/restricted

  // Cache miss: call SMCCall outside the lock — it is a blocking IOKit call and
  // must not be held across it (os_unfair_lock is not designed for blocking
  // sections).
  memset(&inputStructure, 0, sizeof(inputStructure));
  memset(&outputStructure, 0, sizeof(outputStructure));

  inputStructure.key = key;
  inputStructure.data8 = SMC_CMD_READ_KEYINFO;

  result = SMCCall(conn, KERNEL_INDEX_SMC, &inputStructure, &outputStructure);
  if (result != kIOReturnSuccess)
    return result;

  // Check the SMC-layer response code — distinct from the IOKit transport
  // status. Cache the negative result so subsequent lookups for this key skip
  // the IOKit round-trip.
  if (outputStructure.result != 0) {
    os_unfair_lock_lock(&g_keyInfoSpinLock);
    if (g_cachedConn == conn)
      keyInfoCacheInsert(key, NULL);
    os_unfair_lock_unlock(&g_keyInfoSpinLock);
    return kIOReturnError;
  }

  *keyInfo = outputStructure.keyInfo;

  // Re-acquire to insert into the sorted cache. keyInfoCacheInsert handles
  // duplicates internally, so no separate find is needed. Guard against conn
  // switching: if another thread changed g_cachedConn while we were in SMCCall,
  // our result is for the wrong service and must not be cached under the new
  // connection.
  os_unfair_lock_lock(&g_keyInfoSpinLock);
  if (g_cachedConn == conn)
    keyInfoCacheInsert(key, &outputStructure.keyInfo);
  os_unfair_lock_unlock(&g_keyInfoSpinLock);

  return kIOReturnSuccess;
}

kern_return_t SMCReadKey(io_connect_t conn, const UInt32Char_t key,
                         SMCVal_t *val) {
  kern_return_t result;
  SMCKeyData_t inputStructure;
  SMCKeyData_t outputStructure;

  memset(&inputStructure, 0, sizeof(SMCKeyData_t));
  memset(&outputStructure, 0, sizeof(SMCKeyData_t));
  memset(val, 0, sizeof(SMCVal_t));

  inputStructure.key = smcPackKeyBytes(key, 4);
  memcpy(val->key, key, sizeof(val->key));

  result = SMCGetKeyInfo(conn, inputStructure.key, &outputStructure.keyInfo);
  if (result != kIOReturnSuccess)
    return result;

  val->dataSize = outputStructure.keyInfo.dataSize;
  smcUnpackKeyBytes(val->dataType, outputStructure.keyInfo.dataType);

  // Cap read size to the kernel SMC driver's hard per-read limit.
  // Requesting more than SMC_MAX_DATA_SIZE bytes returns kIOReturnBadArgument.
  UInt32 readSize =
      (val->dataSize > SMC_MAX_DATA_SIZE) ? SMC_MAX_DATA_SIZE : val->dataSize;
  inputStructure.keyInfo.dataSize = readSize;
  inputStructure.data8 = SMC_CMD_READ_BYTES;

  result = SMCCall(conn, KERNEL_INDEX_SMC, &inputStructure, &outputStructure);
  if (result != kIOReturnSuccess)
    return result;

  // Check the SMC-layer response code — a non-zero value means the read was
  // rejected by the SMC even though the IOKit transport call succeeded.
  if (outputStructure.result != 0)
    return kIOReturnError;

  memcpy(val->bytes, outputStructure.bytes, readSize);
  val->dataSize = readSize;

  return kIOReturnSuccess;
}

kern_return_t SMCWriteKey(io_connect_t conn, const SMCVal_t *val) {
  SMCVal_t readVal;

  kern_return_t result = SMCReadKey(conn, val->key, &readVal);
  if (result != kIOReturnSuccess)
    return result;

  if (readVal.dataSize != val->dataSize)
    return kIOReturnError;

  return SMCWriteKeyUnsafe(conn, val);
}

kern_return_t SMCWriteKeyUnsafe(io_connect_t conn, const SMCVal_t *val) {
  SMCKeyData_t inputStructure;
  SMCKeyData_t outputStructure;
  kern_return_t result;

  memset(&inputStructure, 0, sizeof(SMCKeyData_t));
  memset(&outputStructure, 0, sizeof(SMCKeyData_t));

  inputStructure.key = smcPackKeyBytes(val->key, 4);
  inputStructure.data8 = SMC_CMD_WRITE_BYTES;

  // Cap write size to SMC_MAX_DATA_SIZE to keep keyInfo.dataSize consistent
  // with the data actually present in the struct; the zeroed remainder is
  // ignored by the driver.
  UInt32 writeSize =
      (val->dataSize > SMC_MAX_DATA_SIZE) ? SMC_MAX_DATA_SIZE : val->dataSize;
  inputStructure.keyInfo.dataSize = writeSize;
  memcpy(inputStructure.bytes, val->bytes, writeSize);

  result = SMCCall(conn, KERNEL_INDEX_SMC, &inputStructure, &outputStructure);
  if (result != kIOReturnSuccess)
    return result;

  // Check the SMC-layer response code — a non-zero value means the write was
  // rejected by the SMC even though the IOKit transport call succeeded.
  return outputStructure.result != 0 ? kIOReturnError : kIOReturnSuccess;
}
