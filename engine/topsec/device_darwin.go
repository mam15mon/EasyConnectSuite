package topsec

/*
#cgo LDFLAGS: -framework IOKit -framework CoreFoundation
#include <IOKit/IOKitLib.h>
#include <CoreFoundation/CoreFoundation.h>
#include <string.h>

static int ecs_topsec_identity(char *buf, size_t size, int *source) {
    io_service_t service = IOServiceGetMatchingService(kIOMainPortDefault, IOServiceMatching("IOPlatformExpertDevice"));
    if (!service) return 1;
    CFTypeRef value = IORegistryEntryCreateCFProperty(service, CFSTR("IOPlatformSerialNumber"), kCFAllocatorDefault, 0);
    *source = 1;
    if (!value) {
        value = IORegistryEntryCreateCFProperty(service, CFSTR("IOPlatformUUID"), kCFAllocatorDefault, 0);
        *source = 2;
    }
    IOObjectRelease(service);
    if (!value) return 2;
    int result = 0;
    if (CFGetTypeID(value) == CFStringGetTypeID()) {
        if (!CFStringGetCString((CFStringRef)value, buf, size, kCFStringEncodingUTF8)) result = 3;
    } else if (CFGetTypeID(value) == CFDataGetTypeID()) {
        CFIndex n = CFDataGetLength((CFDataRef)value);
        if (n <= 0 || n >= size) result = 3;
        else { CFDataGetBytes((CFDataRef)value, CFRangeMake(0, n), (UInt8 *)buf); buf[n] = 0; }
    } else result = 4;
    CFRelease(value);
    if (!result && !buf[0]) result = 3;
    return result;
}
*/
import "C"

import (
	"crypto/md5"
	"encoding/base64"
	"fmt"
	"os"

	vpnlog "EasyConnectSuite/vpncore/log"
)

func localDeviceCode(profileID string) (code string, err error) {
	op := vpnlog.BeginOperation(fmt.Sprintf("[TopSAP pid=%d profile=%q] ", os.Getpid(), profileID), "darwin_device_identity", nil)
	outcome := "interrupted"
	defer func() { op.Finish(outcome, err) }()
	op.Step("read_platform_identity")
	var buffer [256]C.char
	var source C.int
	status := C.ecs_topsec_identity(&buffer[0], C.size_t(len(buffer)), &source)
	if status != 0 {
		err = fmt.Errorf("TopSAP macOS 设备标识读取失败 (stage=%d)", int(status))
		outcome = "failed"
		return "", err
	}
	identity := C.GoString(&buffer[0])
	op.Step("derive_device_code")
	code = darwinDeviceCode(identity)
	outcome = "completed"
	return code, nil
}

func darwinDeviceCode(identity string) string {
	digest := md5.Sum([]byte(identity))
	return base64.StdEncoding.EncodeToString(digest[:])
}
