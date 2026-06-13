package main

import (
	"context"
	"fmt"
	"time"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

const defaultBackgroundTickInterval = time.Minute

func (a *App) startBackgroundWorker(parent context.Context) {
	a.backgroundMu.Lock()
	if a.backgroundCancel != nil {
		a.backgroundCancel()
	}

	ctx, cancel := context.WithCancel(parent)
	a.backgroundCancel = cancel
	a.backgroundWake = make(chan struct{}, 1)
	now := time.Now()
	a.lastScheduledScanRun = now
	a.lastScheduledDatabaseRun = now
	interval := a.backgroundTickInterval
	if interval <= 0 {
		interval = defaultBackgroundTickInterval
	}
	wake := a.backgroundWake
	a.backgroundMu.Unlock()

	go a.backgroundLoop(ctx, interval, wake)
}

func (a *App) stopBackgroundWorker() {
	a.backgroundMu.Lock()
	cancel := a.backgroundCancel
	a.backgroundCancel = nil
	a.backgroundWake = nil
	a.backgroundMu.Unlock()

	if cancel != nil {
		cancel()
	}
}

func (a *App) wakeBackgroundWorker() {
	a.backgroundMu.Lock()
	wake := a.backgroundWake
	a.backgroundMu.Unlock()
	if wake == nil {
		return
	}

	select {
	case wake <- struct{}{}:
	default:
	}
}

func (a *App) backgroundLoop(ctx context.Context, interval time.Duration, wake <-chan struct{}) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.runScheduledWork(ctx, time.Now())
		case <-wake:
			a.runScheduledWork(ctx, time.Now())
		}
	}
}

func (a *App) runScheduledWork(ctx context.Context, now time.Time) {
	a.scheduledWorkMu.Lock()
	defer a.scheduledWorkMu.Unlock()

	settings, err := a.settingsStore.Load()
	if err != nil {
		_ = a.logs().WriteAppLog("error", "讀取背景排程設定失敗："+err.Error())
		return
	}
	if !settings.Background.Enabled {
		return
	}

	a.runScheduledScanIfDue(ctx, settings, now)
	a.runScheduledDatabaseUpdateIfDue(ctx, settings, now)
}

func (a *App) runScheduledScanIfDue(ctx context.Context, settings Settings, now time.Time) {
	if !a.scheduler().ScanDue(settings.ScanSchedule, now, a.lastScheduledScanRun) {
		return
	}

	job, _, decision, err := a.scheduler().RunScheduledScan(ctx, settings.ScanSchedule, settings.PowerPolicy, func(event ScanProgressEvent) {
		if a.ctx != nil {
			wailsruntime.EventsEmit(a.ctx, "scan:progress", event)
		}
	})
	if decision.Defer {
		_ = a.logs().WriteAppLog("info", "排程掃描已延後："+decision.Reason)
		return
	}

	a.lastScheduledScanRun = now
	if err != nil {
		_ = a.logs().WriteAppLog("error", "排程掃描失敗："+err.Error())
		return
	}
	_ = a.logs().WriteAppLog("info", fmt.Sprintf("排程掃描完成：狀態 %s，掃描 %d 筆，偵測 %d 筆，錯誤 %d 筆", job.Status, job.ScannedFiles, job.Detections, job.Errors))
}

func (a *App) runScheduledDatabaseUpdateIfDue(ctx context.Context, settings Settings, now time.Time) {
	if !a.updateSchedulerService().UpdateDue(settings.UpdateSchedule, now, a.lastScheduledDatabaseRun) {
		return
	}

	status, decision, err := a.updateSchedulerService().RunScheduledUpdate(ctx, settings, func(event FreshclamEvent) {
		if a.ctx != nil {
			wailsruntime.EventsEmit(a.ctx, "freshclam:event", event)
		}
	})
	if decision.Defer {
		if decision.Reason == "病毒碼更新由 system updater 管理" {
			a.lastScheduledDatabaseRun = now
		}
		_ = a.logs().WriteAppLog("info", "排程病毒碼更新已延後："+decision.Reason)
		return
	}

	a.lastScheduledDatabaseRun = now
	if err != nil {
		_ = a.logs().WriteAppLog("error", "排程病毒碼更新失敗："+err.Error())
		return
	}
	_ = a.logs().WriteAppLog("info", fmt.Sprintf("排程病毒碼更新完成：版本 %s，簽章 %d 筆", status.Version, status.Signatures))
}
