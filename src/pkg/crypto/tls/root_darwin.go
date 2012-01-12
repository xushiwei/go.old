// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package tls

/*
// Note: We disable -Werror here because the code in this file uses a deprecated API to stay
// compatible with both Mac OS X 10.6 and 10.7. Using a deprecated function on Darwin generates
// a warning.
#cgo CFLAGS: -Wno-error -Wno-deprecated-declarations
#cgo LDFLAGS: -framework CoreFoundation -framework Security
#include <CoreFoundation/CoreFoundation.h>
#include <Security/Security.h>

// FetchPEMRoots fetches the system's list of trusted X.509 root certificates.
//
// On success it returns 0 and fills pemRoots with a CFDataRef that contains the extracted root
// certificates of the system. On failure, the function returns -1.
//
// Note: The CFDataRef returned in pemRoots must be released (using CFRelease) after
// we've consumed its content.
int FetchPEMRoots(CFDataRef *pemRoots) {
	if (pemRoots == NULL) {
		return -1;
	}

	CFArrayRef certs = NULL;
	OSStatus err = SecTrustCopyAnchorCertificates(&certs);
	if (err != noErr) {
		return -1;
	}

	CFMutableDataRef combinedData = CFDataCreateMutable(kCFAllocatorDefault, 0);
	int i, ncerts = CFArrayGetCount(certs);
	for (i = 0; i < ncerts; i++) {
		CFDataRef data = NULL;
		SecCertificateRef cert = (SecCertificateRef)CFArrayGetValueAtIndex(certs, i);
		if (cert == NULL) {
			continue;
		}

		// SecKeychainImportExport is deprecated in >= OS X 10.7, and has been replaced by
		// SecItemExport.  If we're built on a host with a Lion SDK, this code gets conditionally
		// included in the output, also for binaries meant for 10.6.
		//
		// To make sure that we run on both Mac OS X 10.6 and 10.7 we use weak linking
		// and check whether SecItemExport is available before we attempt to call it. On
		// 10.6, this won't be the case, and we'll fall back to calling SecKeychainItemExport.
#if __MAC_OS_X_VERSION_MAX_ALLOWED >= 1070
		if (SecItemExport) {
			err = SecItemExport(cert, kSecFormatX509Cert, kSecItemPemArmour, NULL, &data);
			if (err != noErr) {
				continue;
			}
		} else
#endif
		if (data == NULL) {
			err = SecKeychainItemExport(cert, kSecFormatX509Cert, kSecItemPemArmour, NULL, &data);
			if (err != noErr) {
				continue;
			}
		}

		if (data != NULL) {
			CFDataAppendBytes(combinedData, CFDataGetBytePtr(data), CFDataGetLength(data));
			CFRelease(data);
		}
	}

	CFRelease(certs);

	*pemRoots = combinedData;
	return 0;
}
*/
import "C"
import (
	"crypto/x509"
	"unsafe"
)

func initDefaultRoots() {
	roots := x509.NewCertPool()

	var data C.CFDataRef = nil
	err := C.FetchPEMRoots(&data)
	if err != -1 {
		defer C.CFRelease(C.CFTypeRef(data))
		buf := C.GoBytes(unsafe.Pointer(C.CFDataGetBytePtr(data)), C.int(C.CFDataGetLength(data)))
		roots.AppendCertsFromPEM(buf)
	}

	varDefaultRoots = roots
}
