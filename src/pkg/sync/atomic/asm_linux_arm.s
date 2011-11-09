// Copyright 2011 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Linux/ARM atomic operations.

// Because there is so much variation in ARM devices,
// the Linux kernel provides an appropriate compare-and-swap
// implementation at address 0xffff0fc0.  Caller sets:
//	R0 = old value
//	R1 = new value
//	R2 = valptr
//	LR = return address
// The function returns with CS true if the swap happened.
// http://lxr.linux.no/linux+v2.6.37.2/arch/arm/kernel/entry-armv.S#L850
// On older kernels (before 2.6.24) the function can incorrectly
// report a conflict, so we have to double-check the compare ourselves
// and retry if necessary.
//
// http://git.kernel.org/?p=linux/kernel/git/torvalds/linux-2.6.git;a=commit;h=b49c0f24cf6744a3f4fd09289fe7cade349dead5
//
TEXT cas<>(SB),7,$0
	MOVW	$0xffff0fc0, PC

TEXT ·CompareAndSwapInt32(SB),7,$0
	B	·CompareAndSwapUint32(SB)

// Implement using kernel cas for portability.
TEXT ·CompareAndSwapUint32(SB),7,$0
	MOVW	valptr+0(FP), R2
	MOVW	old+4(FP), R0
casagain:
	MOVW	new+8(FP), R1
	BL cas<>(SB)
	BCC	cascheck
	MOVW	$1, R0
casret:
	MOVW	R0, ret+12(FP)
	RET
cascheck:
	// Kernel lies; double-check.
	MOVW	valptr+0(FP), R2
	MOVW	old+4(FP), R0
	MOVW	0(R2), R3
	CMP	R0, R3
	BEQ	casagain
	MOVW	$0, R0
	B	casret

TEXT ·CompareAndSwapUintptr(SB),7,$0
	B	·CompareAndSwapUint32(SB)

TEXT ·CompareAndSwapPointer(SB),7,$0
	B	·CompareAndSwapUint32(SB)

TEXT ·AddInt32(SB),7,$0
	B	·AddUint32(SB)

// Implement using kernel cas for portability.
TEXT ·AddUint32(SB),7,$0
	MOVW	valptr+0(FP), R2
	MOVW	delta+4(FP), R4
addloop1:
	MOVW	0(R2), R0
	MOVW	R0, R1
	ADD	R4, R1
	BL	cas<>(SB)
	BCC	addloop1
	MOVW	R1, ret+8(FP)
	RET

TEXT ·AddUintptr(SB),7,$0
	B	·AddUint32(SB)

// The kernel provides no 64-bit compare-and-swap,
// so use native ARM instructions, which will only work on
// ARM 11 and later devices.
TEXT ·CompareAndSwapInt64(SB),7,$0
	B	·armCompareAndSwapUint64(SB)

TEXT ·CompareAndSwapUint64(SB),7,$0
	B	·armCompareAndSwapUint64(SB)

TEXT ·AddInt64(SB),7,$0
	B	·armAddUint64(SB)

TEXT ·AddUint64(SB),7,$0
	B	·armAddUint64(SB)

TEXT ·LoadInt32(SB),7,$0
	B	·LoadUint32(SB)

TEXT ·LoadUint32(SB),7,$0
	MOVW	addrptr+0(FP), R2
loadloop1:
	MOVW	0(R2), R0
	MOVW	R0, R1
	BL	cas<>(SB)
	BCC	loadloop1
	MOVW	R1, val+4(FP)
	RET

TEXT ·LoadInt64(SB),7,$0
	B	·armLoadUint64(SB)

TEXT ·LoadUint64(SB),7,$0
	B	·armLoadUint64(SB)

TEXT ·LoadUintptr(SB),7,$0
	B	·LoadUint32(SB)

TEXT ·LoadPointer(SB),7,$0
	B	·LoadUint32(SB)

TEXT ·StoreInt32(SB),7,$0
	B	·StoreUint32(SB)

TEXT ·StoreUint32(SB),7,$0
	MOVW	addrptr+0(FP), R2
	MOVW	val+4(FP), R1
storeloop1:
	MOVW	0(R2), R0
	BL	cas<>(SB)
	BCC	storeloop1
	RET

TEXT ·StoreInt64(SB),7,$0
	B	·armStoreUint64(SB)

TEXT ·StoreUint64(SB),7,$0
	B	·armStoreUint64(SB)

TEXT ·StoreUintptr(SB),7,$0
	B	·StoreUint32(SB)

TEXT ·StorePointer(SB),7,$0
	B	·StoreUint32(SB)
