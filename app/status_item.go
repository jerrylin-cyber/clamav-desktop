package main

import (
	"sync"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

var statusItemState struct {
	sync.Mutex
	app *App
}

func (a *App) startStatusItem() {
	statusItemState.Lock()
	statusItemState.app = a
	statusItemState.Unlock()
	nativeStartStatusItem()
}

func statusItemOpenWindow() {
	app := currentStatusItemApp()
	if app == nil || app.ctx == nil {
		return
	}
	wailsruntime.WindowShow(app.ctx)
	wailsruntime.WindowUnminimise(app.ctx)
}

func statusItemScanDownloads() {
	app := currentStatusItemApp()
	if app == nil {
		return
	}
	go func() {
		_, _, err := app.scanJobs().RunScan(app.context(), []string{downloadsPath()}, ScanOptions{Recursive: true}, func(event ScanProgressEvent) {
			if app.ctx != nil {
				wailsruntime.EventsEmit(app.ctx, "scan:progress", event)
			}
		})
		if err != nil {
			_ = app.logs().WriteAppLog("error", "Status bar 掃描 Downloads 失敗："+err.Error())
			return
		}
		_ = app.logs().WriteAppLog("info", "Status bar 掃描 Downloads 完成")
	}()
}

func statusItemUpdateDatabase() {
	app := currentStatusItemApp()
	if app == nil {
		return
	}
	go func() {
		_, _ = app.UpdateDatabase()
	}()
}

func statusItemPauseSchedule() {
	app := currentStatusItemApp()
	if app == nil {
		return
	}
	settings, err := app.GetSettings()
	if err != nil {
		_ = app.logs().WriteAppLog("error", "Status bar 暫停排程失敗："+err.Error())
		return
	}
	settings.Background.Enabled = false
	if _, err := app.SaveSettings(settings); err != nil {
		_ = app.logs().WriteAppLog("error", "Status bar 暫停排程失敗："+err.Error())
		return
	}
	_ = app.logs().WriteAppLog("info", "Status bar 已暫停背景排程")
}

func statusItemShowLastResult() {
	statusItemOpenWindow()
}

func statusItemQuit() {
	app := currentStatusItemApp()
	if app == nil || app.ctx == nil {
		return
	}
	// 標記為使用者主動結束，讓 beforeClose 不再攔截關閉
	app.quitting.Store(true)
	wailsruntime.Quit(app.ctx)
}

func currentStatusItemApp() *App {
	statusItemState.Lock()
	defer statusItemState.Unlock()
	return statusItemState.app
}
