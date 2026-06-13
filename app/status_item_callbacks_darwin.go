//go:build darwin && cgo

package main

import "C"

//export StatusItemOpenWindow
func StatusItemOpenWindow() {
	statusItemOpenWindow()
}

//export StatusItemScanDownloads
func StatusItemScanDownloads() {
	statusItemScanDownloads()
}

//export StatusItemUpdateDatabase
func StatusItemUpdateDatabase() {
	statusItemUpdateDatabase()
}

//export StatusItemPauseSchedule
func StatusItemPauseSchedule() {
	statusItemPauseSchedule()
}

//export StatusItemShowLastResult
func StatusItemShowLastResult() {
	statusItemShowLastResult()
}

//export StatusItemQuit
func StatusItemQuit() {
	statusItemQuit()
}
