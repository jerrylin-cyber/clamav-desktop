package main

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"
)

func TestParseTimeOfDayValidAndInvalid(t *testing.T) {
	if hour, minute, err := parseTimeOfDay("09:30"); err != nil || hour != 9 || minute != 30 {
		t.Fatalf("unexpected result: hour=%d minute=%d err=%v", hour, minute, err)
	}

	for _, value := range []string{"", "9", "24:00", "12:60", "ab:cd"} {
		if _, _, err := parseTimeOfDay(value); err == nil {
			t.Errorf("expected error for %q, got nil", value)
		}
	}
}

func TestNextScheduledTimeDaily(t *testing.T) {
	before := time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC)
	next, err := nextScheduledTime(before, "daily", "12:00", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Errorf("expected %v, got %v", want, next)
	}

	after := time.Date(2026, 6, 12, 13, 0, 0, 0, time.UTC)
	next, err = nextScheduledTime(after, "daily", "12:00", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want = time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Errorf("expected %v, got %v", want, next)
	}

	exact := time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC)
	next, err = nextScheduledTime(exact, "daily", "12:00", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want = time.Date(2026, 6, 13, 12, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Errorf("expected next occurrence to roll over when equal, got %v", next)
	}
}

func TestNextScheduledTimeWeekly(t *testing.T) {
	// 2026-06-12 is a Friday (weekday=5).
	friday := time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC)

	// Next Monday (weekday=1) at 09:00.
	next, err := nextScheduledTime(friday, "weekly", "09:00", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Errorf("expected %v, got %v", want, next)
	}

	// Same weekday, but the time-of-day has already passed today: roll to next week.
	next, err = nextScheduledTime(friday, "weekly", "09:00", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want = time.Date(2026, 6, 19, 9, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Errorf("expected %v, got %v", want, next)
	}

	// Same weekday, time-of-day still ahead today.
	next, err = nextScheduledTime(friday, "weekly", "18:00", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want = time.Date(2026, 6, 12, 18, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Errorf("expected %v, got %v", want, next)
	}
}

func TestNextScheduledTimeRejectsInvalidInput(t *testing.T) {
	now := time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC)

	if _, err := nextScheduledTime(now, "monthly", "12:00", 0); err == nil {
		t.Errorf("expected error for unsupported frequency")
	}
	if _, err := nextScheduledTime(now, "weekly", "12:00", 7); err == nil {
		t.Errorf("expected error for invalid weekday")
	}
	if _, err := nextScheduledTime(now, "daily", "bad", 0); err == nil {
		t.Errorf("expected error for invalid time-of-day")
	}
}

func TestNextScanRunReturnsFalseWhenDisabled(t *testing.T) {
	svc := newSchedulerService(nil, nil)
	schedule := ScanSchedule{Enabled: false, Frequency: "daily", TimeOfDay: "12:00"}

	if _, ok := svc.NextScanRun(schedule, time.Now()); ok {
		t.Errorf("expected disabled schedule to return ok=false")
	}
}

func TestScanDueCatchesUpAfterMissedRun(t *testing.T) {
	svc := newSchedulerService(nil, nil)
	schedule := ScanSchedule{Enabled: true, Frequency: "daily", TimeOfDay: "12:00"}

	lastRun := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)

	// Only a few hours later: next run (2026-06-11 12:00) hasn't arrived yet.
	soon := time.Date(2026, 6, 10, 18, 0, 0, 0, time.UTC)
	if svc.ScanDue(schedule, soon, lastRun) {
		t.Errorf("expected ScanDue=false before the next trigger time")
	}

	// App was closed and reopened two days later: the missed 06-11 run is due.
	later := time.Date(2026, 6, 12, 9, 0, 0, 0, time.UTC)
	if !svc.ScanDue(schedule, later, lastRun) {
		t.Errorf("expected ScanDue=true to catch up on the missed run")
	}
}

func TestScanDueFalseWhenDisabled(t *testing.T) {
	svc := newSchedulerService(nil, nil)
	schedule := ScanSchedule{Enabled: false, Frequency: "daily", TimeOfDay: "12:00"}

	lastRun := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	now := time.Date(2026, 6, 12, 0, 0, 0, 0, time.UTC)
	if svc.ScanDue(schedule, now, lastRun) {
		t.Errorf("expected paused schedule to never be due")
	}
}

func TestRunScheduledScanDefersPerPowerPolicy(t *testing.T) {
	policySvc := &PowerPolicyService{
		run: func(ctx context.Context, name string, args ...string) (string, error) {
			if len(args) > 0 && args[len(args)-1] == "batt" {
				return samplePmsetBattBattery, nil
			}
			return samplePmsetGenLowPowerOff, nil
		},
	}
	svc := newSchedulerService(nil, policySvc)
	schedule := ScanSchedule{Paths: []string{"/tmp"}}
	policy := PowerPolicy{RunOnBattery: false, RunInLowPowerMode: true}

	job, results, decision, err := svc.RunScheduledScan(context.Background(), schedule, policy, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !decision.Defer {
		t.Errorf("expected Defer=true on battery with RunOnBattery=false")
	}
	if job.ID != "" || results != nil {
		t.Errorf("expected no scan to run when deferred, got job=%#v results=%#v", job, results)
	}
}

func TestRunScheduledScanReturnsPowerPolicyError(t *testing.T) {
	policySvc := &PowerPolicyService{
		run: func(ctx context.Context, name string, args ...string) (string, error) {
			return "", errors.New("pmset not found")
		},
	}
	svc := newSchedulerService(nil, policySvc)
	schedule := ScanSchedule{Paths: []string{"/tmp"}}

	if _, _, _, err := svc.RunScheduledScan(context.Background(), schedule, PowerPolicy{}, nil); err == nil {
		t.Errorf("expected error when power policy check fails")
	}
}

func TestRunScheduledScanRunsWhenNotDeferred(t *testing.T) {
	policySvc := &PowerPolicyService{
		run: func(ctx context.Context, name string, args ...string) (string, error) {
			if len(args) > 0 && args[len(args)-1] == "batt" {
				return samplePmsetBattAC, nil
			}
			return samplePmsetGenLowPowerOff, nil
		},
	}

	file := writeScanFixture(t, "clean.txt", "clean")
	manager := testScanJobManager(t, func(_ context.Context, path string) (string, error) {
		if path != file {
			t.Fatalf("unexpected scan path: %s", path)
		}
		return "stream: OK", nil
	})

	svc := newSchedulerService(manager, policySvc)
	schedule := ScanSchedule{Paths: []string{file}}
	policy := PowerPolicy{RunOnBattery: false, RunInLowPowerMode: false}

	job, results, decision, err := svc.RunScheduledScan(context.Background(), schedule, policy, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if decision.Defer {
		t.Errorf("expected Defer=false on AC power")
	}
	if job.Status != "completed" {
		t.Errorf("unexpected job status: %s", job.Status)
	}
	if len(results) != 1 || results[0].Status != "clean" {
		t.Errorf("unexpected results: %#v", results)
	}
}

func TestNextUpdateRunAppliesDeterministicJitter(t *testing.T) {
	svc := newUpdateSchedulerService(nil, nil)
	svc.identity = func() string { return "host-a\n/Users/jerry" }

	after := time.Date(2026, 6, 12, 2, 59, 0, 0, time.UTC)
	schedule := UpdateSchedule{Enabled: true, Frequency: "daily", TimeOfDay: "03:00"}

	next, ok := svc.NextUpdateRun(schedule, after)
	if !ok {
		t.Fatal("expected enabled update schedule")
	}

	jitter := deterministicUpdateJitter("host-a\n/Users/jerry")
	want := time.Date(2026, 6, 12, 3, 0, 0, 0, time.UTC).Add(jitter)
	if !next.Equal(want) {
		t.Fatalf("expected %v, got %v", want, next)
	}
}

func TestNextUpdateRunRollsAfterJitteredTime(t *testing.T) {
	svc := newUpdateSchedulerService(nil, nil)
	svc.identity = func() string { return "host-a\n/Users/jerry" }

	jitter := deterministicUpdateJitter("host-a\n/Users/jerry")
	after := time.Date(2026, 6, 12, 3, 0, 0, 0, time.UTC).Add(jitter)
	schedule := UpdateSchedule{Enabled: true, Frequency: "daily", TimeOfDay: "03:00"}

	next, ok := svc.NextUpdateRun(schedule, after)
	if !ok {
		t.Fatal("expected enabled update schedule")
	}

	want := time.Date(2026, 6, 13, 3, 0, 0, 0, time.UTC).Add(jitter)
	if !next.Equal(want) {
		t.Fatalf("expected %v, got %v", want, next)
	}
}

func TestRunScheduledUpdateDefersSystemSharedRuntime(t *testing.T) {
	freshclam := &FreshclamService{
		run: func(ctx context.Context, command string, args []string, stdout io.Writer, stderr io.Writer) error {
			t.Fatal("freshclam should not run for system-shared runtime")
			return nil
		},
	}
	svc := newUpdateSchedulerService(freshclam, newPowerPolicyService())
	settings := defaultSettings()
	settings.RuntimeMode = "system-shared"

	status, decision, err := svc.RunScheduledUpdate(context.Background(), settings, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !decision.Defer || decision.Reason == "" {
		t.Fatalf("expected system updater defer decision, got %#v", decision)
	}
	if status.Path != "" {
		t.Fatalf("expected empty status when deferred, got %#v", status)
	}
}

func TestRunScheduledUpdateDefersPerPowerPolicy(t *testing.T) {
	policySvc := &PowerPolicyService{
		run: func(ctx context.Context, name string, args ...string) (string, error) {
			if len(args) > 0 && args[len(args)-1] == "batt" {
				return samplePmsetBattBattery, nil
			}
			return samplePmsetGenLowPowerOff, nil
		},
	}
	svc := newUpdateSchedulerService(&FreshclamService{}, policySvc)
	settings := defaultSettings()
	settings.RuntimeMode = "external"
	settings.PowerPolicy.RunOnBattery = false

	_, decision, err := svc.RunScheduledUpdate(context.Background(), settings, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !decision.Defer {
		t.Fatal("expected update to be deferred on battery")
	}
}
