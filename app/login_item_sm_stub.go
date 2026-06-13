//go:build !darwin || !cgo

package main

import "errors"

func registerSMAppService() error {
	return errors.New("SMAppService 僅支援 macOS cgo build")
}

func unregisterSMAppService() error {
	return nil
}

func smAppServiceStatus() LoginItemStatus {
	return LoginItemStatus{Method: "SMAppService"}
}
