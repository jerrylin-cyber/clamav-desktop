package main

import (
	"context"
	"fmt"
	"hash/fnv"
	"os"
	"strconv"
	"strings"
	"time"
)

// SchedulerService computes scan schedule trigger times and runs scheduled
// scans, honoring pause state (ScanSchedule.Enabled), power policy deferral
// (PowerPolicyService), and missed-run catch-up.
type SchedulerService struct {
	scanJobManager *ScanJobManager
	powerPolicy    *PowerPolicyService
}

func newSchedulerService(scanJobManager *ScanJobManager, powerPolicy *PowerPolicyService) *SchedulerService {
	return &SchedulerService{scanJobManager: scanJobManager, powerPolicy: powerPolicy}
}

// NextScanRun returns the next time a scheduled scan should run strictly
// after `after`, and whether the schedule is enabled and valid.
func (s *SchedulerService) NextScanRun(schedule ScanSchedule, after time.Time) (time.Time, bool) {
	if !schedule.Enabled {
		return time.Time{}, false
	}
	next, err := nextScheduledTime(after, schedule.Frequency, schedule.TimeOfDay, schedule.Weekday)
	if err != nil {
		return time.Time{}, false
	}
	return next, true
}

// ScanDue reports whether a scheduled scan is due: the schedule is enabled
// and the trigger time computed from lastRun has arrived. If the app was
// closed when that trigger time passed, ScanDue keeps returning true until
// the scan runs, providing missed-run catch-up.
func (s *SchedulerService) ScanDue(schedule ScanSchedule, now time.Time, lastRun time.Time) bool {
	next, ok := s.NextScanRun(schedule, lastRun)
	if !ok {
		return false
	}
	return !now.Before(next)
}

// RunScheduledScan runs the scheduled scan unless the user's power policy
// says it should be deferred. Callers should call RunScheduledScan again on
// the next tick so a deferred scan runs once the machine is plugged in or Low
// Power Mode is turned off (see PowerPolicyService.ShouldDefer).
func (s *SchedulerService) RunScheduledScan(ctx context.Context, schedule ScanSchedule, policy PowerPolicy, emit func(ScanProgressEvent)) (ScanJob, []ScanResult, DeferDecision, error) {
	decision, err := s.powerPolicy.ShouldDefer(ctx, policy)
	if err != nil {
		return ScanJob{}, nil, DeferDecision{}, err
	}
	if decision.Defer {
		return ScanJob{}, nil, decision, nil
	}

	job, results, err := s.scanJobManager.RunScan(ctx, schedule.Paths, ScanOptions{Recursive: true}, emit)
	return job, results, DeferDecision{}, err
}

// UpdateSchedulerService computes database update trigger times and runs
// scheduled freshclam updates for modes where the user app owns updates.
type UpdateSchedulerService struct {
	freshclam   *FreshclamService
	powerPolicy *PowerPolicyService
	identity    func() string
}

func newUpdateSchedulerService(freshclam *FreshclamService, powerPolicy *PowerPolicyService) *UpdateSchedulerService {
	return &UpdateSchedulerService{freshclam: freshclam, powerPolicy: powerPolicy}
}

// NextUpdateRun 回傳 after 之後下一次病毒碼更新的觸發時間，並加入依機器身分決定的固定 jitter 以分散伺服器負載；
// schedule 未啟用或無效時回傳 false。
func (s *UpdateSchedulerService) NextUpdateRun(schedule UpdateSchedule, after time.Time) (time.Time, bool) {
	if !schedule.Enabled {
		return time.Time{}, false
	}

	jitter := deterministicUpdateJitter(s.identityValue())
	next, err := nextScheduledTime(after.Add(-jitter), schedule.Frequency, schedule.TimeOfDay, 0)
	if err != nil {
		return time.Time{}, false
	}
	return next.Add(jitter), true
}

// UpdateDue 回報病毒碼更新是否到期：schedule 已啟用且依 lastRun 推算的觸發時間已過。
func (s *UpdateSchedulerService) UpdateDue(schedule UpdateSchedule, now time.Time, lastRun time.Time) bool {
	next, ok := s.NextUpdateRun(schedule, lastRun)
	if !ok {
		return false
	}
	return !now.Before(next)
}

// RunScheduledUpdate 執行排程病毒碼更新。共用執行環境（system-shared）的更新由系統管理而直接延後；
// 其餘情況依電源政策決定是否延後，否則呼叫 freshclam 更新。
func (s *UpdateSchedulerService) RunScheduledUpdate(ctx context.Context, settings Settings, emit func(FreshclamEvent)) (DatabaseStatus, DeferDecision, error) {
	if settings.RuntimeMode == "system-shared" {
		return DatabaseStatus{}, DeferDecision{Defer: true, Reason: "病毒碼更新由 system updater 管理"}, nil
	}

	decision, err := s.powerPolicy.ShouldDefer(ctx, settings.PowerPolicy)
	if err != nil {
		return DatabaseStatus{}, DeferDecision{}, err
	}
	if decision.Defer {
		return DatabaseStatus{}, decision, nil
	}

	status, err := s.freshclam.UpdateDatabase(ctx, emit)
	return status, DeferDecision{}, err
}

func (s *UpdateSchedulerService) identityValue() string {
	if s.identity != nil {
		return s.identity()
	}

	hostname, _ := os.Hostname()
	userHome, _ := os.UserHomeDir()
	return hostname + "\n" + userHome
}

func deterministicUpdateJitter(identity string) time.Duration {
	if strings.TrimSpace(identity) == "" {
		return 0
	}

	hash := fnv.New32a()
	_, _ = hash.Write([]byte(identity))
	return time.Duration(hash.Sum32()%1800) * time.Second
}

// nextScheduledTime computes the next time strictly after `after` that
// matches the given frequency ("daily" or "weekly"), time-of-day ("HH:MM"),
// and (for weekly frequency) weekday (0=Sunday..6=Saturday, matching
// time.Weekday).
func nextScheduledTime(after time.Time, frequency string, timeOfDay string, weekday int) (time.Time, error) {
	hour, minute, err := parseTimeOfDay(timeOfDay)
	if err != nil {
		return time.Time{}, err
	}

	candidate := time.Date(after.Year(), after.Month(), after.Day(), hour, minute, 0, 0, after.Location())
	if !candidate.After(after) {
		candidate = candidate.AddDate(0, 0, 1)
	}

	switch frequency {
	case "daily", "":
		return candidate, nil
	case "weekly":
		if weekday < 0 || weekday > 6 {
			return time.Time{}, fmt.Errorf("無效的星期: %d", weekday)
		}
		for candidate.Weekday() != time.Weekday(weekday) {
			candidate = candidate.AddDate(0, 0, 1)
		}
		return candidate, nil
	default:
		return time.Time{}, fmt.Errorf("不支援的排程頻率: %s", frequency)
	}
}

func parseTimeOfDay(value string) (int, int, error) {
	parts := strings.Split(value, ":")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("無效的時間格式: %s", value)
	}
	hour, err := strconv.Atoi(parts[0])
	if err != nil || hour < 0 || hour > 23 {
		return 0, 0, fmt.Errorf("無效的時間格式: %s", value)
	}
	minute, err := strconv.Atoi(parts[1])
	if err != nil || minute < 0 || minute > 59 {
		return 0, 0, fmt.Errorf("無效的時間格式: %s", value)
	}
	return hour, minute, nil
}
