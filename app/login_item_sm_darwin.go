//go:build darwin && cgo

package main

/*
#cgo darwin CFLAGS: -x objective-c -fobjc-arc
#cgo darwin LDFLAGS: -framework Foundation -framework ServiceManagement
#import <Foundation/Foundation.h>
#import <ServiceManagement/ServiceManagement.h>
#include <stdlib.h>
#include <string.h>

static char *copyNSString(NSString *value) {
	if (value == nil) {
		return NULL;
	}
	const char *utf8 = [value UTF8String];
	if (utf8 == NULL) {
		return NULL;
	}
	return strdup(utf8);
}

static bool RegisterSMMainApp(char **errorOut) {
	@autoreleasepool {
		if (@available(macOS 13.0, *)) {
			NSError *error = nil;
			BOOL ok = [[SMAppService mainAppService] registerAndReturnError:&error];
			if (!ok && errorOut != NULL) {
				*errorOut = copyNSString([error localizedDescription]);
			}
			return ok;
		}
		if (errorOut != NULL) {
			*errorOut = strdup("SMAppService 需要 macOS 13 或更新版本");
		}
		return false;
	}
}

static bool UnregisterSMMainApp(char **errorOut) {
	@autoreleasepool {
		if (@available(macOS 13.0, *)) {
			NSError *error = nil;
			BOOL ok = [[SMAppService mainAppService] unregisterAndReturnError:&error];
			if (!ok && errorOut != NULL) {
				*errorOut = copyNSString([error localizedDescription]);
			}
			return ok;
		}
		if (errorOut != NULL) {
			*errorOut = strdup("SMAppService 需要 macOS 13 或更新版本");
		}
		return false;
	}
}

static int SMMainAppStatus(char **errorOut) {
	@autoreleasepool {
		if (@available(macOS 13.0, *)) {
			return (int)[[SMAppService mainAppService] status];
		}
		if (errorOut != NULL) {
			*errorOut = strdup("SMAppService 需要 macOS 13 或更新版本");
		}
		return -1;
	}
}
*/
import "C"

import (
	"errors"
	"unsafe"
)

func registerSMAppService() error {
	var cErr *C.char
	if C.RegisterSMMainApp(&cErr) {
		return nil
	}
	return smError(cErr)
}

func unregisterSMAppService() error {
	var cErr *C.char
	if C.UnregisterSMMainApp(&cErr) {
		return nil
	}
	return smError(cErr)
}

func smAppServiceStatus() LoginItemStatus {
	var cErr *C.char
	status := int(C.SMMainAppStatus(&cErr))
	if cErr != nil {
		err := C.GoString(cErr)
		C.free(unsafe.Pointer(cErr))
		return LoginItemStatus{Method: "SMAppService", Error: err}
	}
	switch status {
	case 1:
		return LoginItemStatus{Enabled: true, Method: "SMAppService"}
	case 2:
		return LoginItemStatus{Method: "SMAppService", Error: "SMAppService 需要使用者在 System Settings 核准"}
	case 0, 3:
		return LoginItemStatus{Method: "SMAppService"}
	default:
		return LoginItemStatus{Method: "SMAppService", Error: "未知 SMAppService 狀態"}
	}
}

func smError(cErr *C.char) error {
	if cErr == nil {
		return errors.New("SMAppService 操作失敗")
	}
	err := C.GoString(cErr)
	C.free(unsafe.Pointer(cErr))
	return errors.New(err)
}
