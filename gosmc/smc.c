/*
 * Apple System Management Control (SMC) Tool
 * Copyright (C) 2006 devnull
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
 * Foundation, Inc., 51 Franklin Street, Fifth Floor, Boston, MA  02110-1301, USA.
 */
/*
cc ./smc.c  -o smcutil -framework IOKit -framework CoreFoundation -Wno-four-char-constants -Wall -g -arch i386
 */

#include <stdio.h>
#include <IOKit/IOKitLib.h>
#include <Kernel/string.h>
#include <libkern/OSAtomic.h>
#include <os/lock.h>

#include "smc.h"

// Cache SMC key info to reduce IOKit round-trips.
// 2048 entries covers all keys on current and near-future Apple Silicon Macs (M4/M5 report ~1800 keys).
#define KEY_INFO_CACHE_SIZE 2048

typedef struct {
    UInt32               key;
    SMCKeyData_keyInfo_t keyInfo;
} SMCKeyInfoCacheEntry_t;

SMCKeyInfoCacheEntry_t  g_keyInfoCache[KEY_INFO_CACHE_SIZE];
int                     g_keyInfoCacheCount = 0;
int                     g_keyInfoCacheDirty = 0;
os_unfair_lock          g_keyInfoSpinLock = OS_UNFAIR_LOCK_INIT;

// smcPackKeyBytes packs a 4-character SMC key string into a big-endian UInt32.
// size is clamped to 4 to prevent shift UB (C99 §6.5.7: shifting ≥32 bits is undefined).
static UInt32 smcPackKeyBytes(const char *str, int size)
{
    UInt32 total = 0;
    int i;

    if (size > 4) size = 4;

    for (i = 0; i < size; i++)
        total += (UInt32)(unsigned char)str[i] << ((size - 1 - i) * 8);

    return total;
}

// smcUnpackKeyBytes converts a big-endian UInt32 key code back to a 4-character null-terminated string.
static void smcUnpackKeyBytes(char *str, UInt32 val)
{
    snprintf(str, 5, "%c%c%c%c",
             (unsigned int) val >> 24,
             (unsigned int) val >> 16,
             (unsigned int) val >> 8,
             (unsigned int) val);
}

kern_return_t SMCOpen(const char *serviceName, io_connect_t *conn)
{
    kern_return_t result;
    io_iterator_t iterator;
    io_object_t   device;

    CFMutableDictionaryRef matchingDictionary = IOServiceMatching(serviceName);
    if (!matchingDictionary)
        return kIOReturnNoMemory;

    result = IOServiceGetMatchingServices(kIOMainPortDefault, matchingDictionary, &iterator);
    if (result != kIOReturnSuccess)
    {
        //printf("Error: IOServiceGetMatchingServices() = %08x\n", result);
        return result;
    }

    device = IOIteratorNext(iterator);
    IOObjectRelease((io_object_t)iterator);
    if (device == 0)
    {
        //printf("Error: no SMC found\n");
        return kIOReturnNoDevice;
    }

    result = IOServiceOpen(device, mach_task_self(), 0, conn);
    IOObjectRelease(device);
    if (result != kIOReturnSuccess)
    {
        //printf("Error: IOServiceOpen() = %08x\n", result);
        return result;
    }

    return kIOReturnSuccess;
}

kern_return_t SMCClose(io_connect_t conn)
{
    return IOServiceClose(conn);
}

kern_return_t SMCCall(io_connect_t conn, int index, SMCKeyData_t *inputStructure, SMCKeyData_t *outputStructure)
{
    size_t   structureInputSize;
    size_t   structureOutputSize;

    structureInputSize = sizeof(SMCKeyData_t);
    structureOutputSize = sizeof(SMCKeyData_t);

 	return IOConnectCallStructMethod(
									 conn,
									 index,
									 inputStructure,
									 structureInputSize,
									 outputStructure,
									 &structureOutputSize
									 );
}

// cacheEntryCompare is the qsort comparator for SMCKeyInfoCacheEntry_t, sorted ascending by key.
static int cacheEntryCompare(const void *a, const void *b)
{
    const SMCKeyInfoCacheEntry_t *ea = (const SMCKeyInfoCacheEntry_t *)a;
    const SMCKeyInfoCacheEntry_t *eb = (const SMCKeyInfoCacheEntry_t *)b;

    if (ea->key < eb->key) return -1;
    if (ea->key > eb->key) return  1;

    return 0;
}

// keyInfoCacheFind sorts the cache (lazily, on first find after a dirty insert) then
// performs a binary search. Writes the found entry to *keyInfo when keyInfo is non-NULL.
// Returns 1 on hit, 0 on miss. Must be called with g_keyInfoSpinLock held.
static int keyInfoCacheFind(UInt32 key, SMCKeyData_keyInfo_t *keyInfo)
{
    if (g_keyInfoCacheDirty)
    {
        qsort(g_keyInfoCache, g_keyInfoCacheCount, sizeof(SMCKeyInfoCacheEntry_t), cacheEntryCompare);
        g_keyInfoCacheDirty = 0;
    }

    int lo = 0, hi = g_keyInfoCacheCount - 1;

    while (lo <= hi)
    {
        int mid = lo + (hi - lo) / 2;

        if (g_keyInfoCache[mid].key == key)
        {
            if (keyInfo)
                *keyInfo = g_keyInfoCache[mid].keyInfo;
            return 1;
        }

        if (g_keyInfoCache[mid].key < key)
            lo = mid + 1;
        else
            hi = mid - 1;
    }

    return 0;
}

// keyInfoCacheInsert appends a new entry and marks the cache dirty for lazy sorting.
// Must be called with g_keyInfoSpinLock held. No-op when the cache is full.
static void keyInfoCacheInsert(UInt32 key, const SMCKeyData_keyInfo_t *keyInfo)
{
    if (g_keyInfoCacheCount >= KEY_INFO_CACHE_SIZE)
        return;

    g_keyInfoCache[g_keyInfoCacheCount].key     = key;
    g_keyInfoCache[g_keyInfoCacheCount].keyInfo = *keyInfo;
    ++g_keyInfoCacheCount;
    g_keyInfoCacheDirty = 1;
}

// SMCGetKeyInfo returns key metadata, using a sorted cache to reduce IOKit round-trips.
// The lock is held only for short cache operations; the blocking SMCCall runs lock-free.
kern_return_t SMCGetKeyInfo(io_connect_t conn, UInt32 key, SMCKeyData_keyInfo_t* keyInfo)
{
    SMCKeyData_t  inputStructure;
    SMCKeyData_t  outputStructure;
    kern_return_t result;

    // Fast path: binary-search the sorted cache under the lock (no blocking I/O).
    os_unfair_lock_lock(&g_keyInfoSpinLock);
    int found = keyInfoCacheFind(key, keyInfo);
    os_unfair_lock_unlock(&g_keyInfoSpinLock);

    if (found)
        return kIOReturnSuccess;

    // Cache miss: call SMCCall outside the lock — it is a blocking IOKit call and
    // must not be held across it (os_unfair_lock is not designed for blocking sections).
    memset(&inputStructure, 0, sizeof(inputStructure));
    memset(&outputStructure, 0, sizeof(outputStructure));

    inputStructure.key   = key;
    inputStructure.data8 = SMC_CMD_READ_KEYINFO;

    result = SMCCall(conn, KERNEL_INDEX_SMC, &inputStructure, &outputStructure);
    if (result != kIOReturnSuccess)
        return result;

    *keyInfo = outputStructure.keyInfo;

    // Re-acquire to insert into the sorted cache. Re-check for duplicates in case
    // another thread looked up the same key concurrently between our two lock windows.
    os_unfair_lock_lock(&g_keyInfoSpinLock);
    if (!keyInfoCacheFind(key, NULL))
        keyInfoCacheInsert(key, &outputStructure.keyInfo);
    os_unfair_lock_unlock(&g_keyInfoSpinLock);

    return kIOReturnSuccess;
}

kern_return_t SMCReadKey(io_connect_t conn, const UInt32Char_t key, SMCVal_t *val)
{
    kern_return_t result;
    SMCKeyData_t  inputStructure;
    SMCKeyData_t  outputStructure;

    memset(&inputStructure, 0, sizeof(SMCKeyData_t));
    memset(&outputStructure, 0, sizeof(SMCKeyData_t));
    memset(val, 0, sizeof(SMCVal_t));

    inputStructure.key = smcPackKeyBytes(key, 4);
    //REVEIW_REHABMAN: mempcy used to avoid deprecated strcpy...
    //strcpy(val->key, key);
    memcpy(val->key, key, sizeof(val->key));

    result = SMCGetKeyInfo(conn, inputStructure.key, &outputStructure.keyInfo);
    if (result != kIOReturnSuccess)
        return result;

    val->dataSize = outputStructure.keyInfo.dataSize;
    smcUnpackKeyBytes(val->dataType, outputStructure.keyInfo.dataType);

    // Cap read size to the kernel SMC driver's hard per-read limit.
    // Requesting more than SMC_MAX_DATA_SIZE bytes returns kIOReturnBadArgument.
    UInt32 readSize = (val->dataSize > SMC_MAX_DATA_SIZE) ? SMC_MAX_DATA_SIZE : val->dataSize;
    inputStructure.keyInfo.dataSize = readSize;
    inputStructure.data8 = SMC_CMD_READ_BYTES;

    result = SMCCall(conn, KERNEL_INDEX_SMC, &inputStructure, &outputStructure);
    if (result != kIOReturnSuccess)
        return result;

    memcpy(val->bytes, outputStructure.bytes, readSize);
    val->dataSize = readSize;

    return kIOReturnSuccess;
}

kern_return_t SMCWriteKey(io_connect_t conn, const SMCVal_t *val)
{
    SMCVal_t      readVal;

    IOReturn result = SMCReadKey(conn, val->key, &readVal);
    if (result != kIOReturnSuccess)
        return result;

    if (readVal.dataSize != val->dataSize)
        return kIOReturnError;

    return SMCWriteKeyUnsafe(conn, val);
}

kern_return_t SMCWriteKeyUnsafe(io_connect_t conn, const SMCVal_t *val)
{
    SMCKeyData_t  inputStructure;
    SMCKeyData_t  outputStructure;

    memset(&inputStructure, 0, sizeof(SMCKeyData_t));
    memset(&outputStructure, 0, sizeof(SMCKeyData_t));

    inputStructure.key = smcPackKeyBytes(val->key, 4);
    inputStructure.data8 = SMC_CMD_WRITE_BYTES;
    inputStructure.keyInfo.dataSize = val->dataSize;
    memcpy(inputStructure.bytes, val->bytes, sizeof(val->bytes));

    return SMCCall(conn, KERNEL_INDEX_SMC, &inputStructure, &outputStructure);
}
