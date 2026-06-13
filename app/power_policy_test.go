package main

import (
	"context"
	"errors"
	"testing"
)

const samplePmsetBattAC = `Now drawing from 'AC Power'
 -InternalBattery-0 (id=4325507)	100%; charged; 0:00 remaining present: true
`

const samplePmsetBattBattery = `Now drawing from 'Battery Power'
 -InternalBattery-0 (id=4325507)	72%; discharging; 3:12 remaining present: true
`

const samplePmsetGenLowPowerOn = `System-wide power settings:
Currently in use:
 standbydelaylow	10
 standby	1
 lowpowermode	1
 womp	1
`

const samplePmsetGenLowPowerOff = `System-wide power settings:
Currently in use:
 standbydelaylow	10
 standby	1
 lowpowermode	0
 womp	1
`

func TestParsePowerStatusOnACWithLowPowerModeOff(t *testing.T) {
	status := parsePowerStatus(samplePmsetBattAC, samplePmsetGenLowPowerOff)

	if status.OnBattery {
		t.Errorf("expected OnBattery=false, got true")
	}
	if status.LowPowerMode {
		t.Errorf("expected LowPowerMode=false, got true")
	}
}

func TestParsePowerStatusOnBatteryWithLowPowerModeOn(t *testing.T) {
	status := parsePowerStatus(samplePmsetBattBattery, samplePmsetGenLowPowerOn)

	if !status.OnBattery {
		t.Errorf("expected OnBattery=true, got false")
	}
	if !status.LowPowerMode {
		t.Errorf("expected LowPowerMode=true, got false")
	}
}

func TestReadStatusUsesRunner(t *testing.T) {
	svc := &PowerPolicyService{
		run: func(ctx context.Context, name string, args ...string) (string, error) {
			if len(args) > 0 && args[len(args)-1] == "batt" {
				return samplePmsetBattBattery, nil
			}
			return samplePmsetGenLowPowerOn, nil
		},
	}

	status, err := svc.ReadStatus(context.Background())
	if err != nil {
		t.Fatalf("ReadStatus returned error: %v", err)
	}
	if !status.OnBattery || !status.LowPowerMode {
		t.Errorf("unexpected status: %+v", status)
	}
}

func TestReadStatusReturnsErrorWhenPmsetFails(t *testing.T) {
	svc := &PowerPolicyService{
		run: func(ctx context.Context, name string, args ...string) (string, error) {
			return "", errors.New("pmset not found")
		},
	}

	if _, err := svc.ReadStatus(context.Background()); err == nil {
		t.Errorf("expected error when pmset fails, got nil")
	}
}

func TestShouldDeferOnBatteryWhenRunOnBatteryDisabled(t *testing.T) {
	svc := &PowerPolicyService{
		run: func(ctx context.Context, name string, args ...string) (string, error) {
			if len(args) > 0 && args[len(args)-1] == "batt" {
				return samplePmsetBattBattery, nil
			}
			return samplePmsetGenLowPowerOff, nil
		},
	}

	policy := PowerPolicy{RunOnBattery: false, RunInLowPowerMode: true}

	decision, err := svc.ShouldDefer(context.Background(), policy)
	if err != nil {
		t.Fatalf("ShouldDefer returned error: %v", err)
	}
	if !decision.Defer {
		t.Errorf("expected Defer=true when on battery and RunOnBattery=false")
	}
	if decision.Reason == "" {
		t.Errorf("expected a non-empty reason")
	}
}

func TestShouldDeferInLowPowerModeWhenRunInLowPowerModeDisabled(t *testing.T) {
	svc := &PowerPolicyService{
		run: func(ctx context.Context, name string, args ...string) (string, error) {
			if len(args) > 0 && args[len(args)-1] == "batt" {
				return samplePmsetBattAC, nil
			}
			return samplePmsetGenLowPowerOn, nil
		},
	}

	policy := PowerPolicy{RunOnBattery: true, RunInLowPowerMode: false}

	decision, err := svc.ShouldDefer(context.Background(), policy)
	if err != nil {
		t.Fatalf("ShouldDefer returned error: %v", err)
	}
	if !decision.Defer {
		t.Errorf("expected Defer=true when in Low Power Mode and RunInLowPowerMode=false")
	}
}

func TestShouldDeferNotDeferredWhenPolicyAllows(t *testing.T) {
	svc := &PowerPolicyService{
		run: func(ctx context.Context, name string, args ...string) (string, error) {
			if len(args) > 0 && args[len(args)-1] == "batt" {
				return samplePmsetBattBattery, nil
			}
			return samplePmsetGenLowPowerOn, nil
		},
	}

	policy := PowerPolicy{RunOnBattery: true, RunInLowPowerMode: true}

	decision, err := svc.ShouldDefer(context.Background(), policy)
	if err != nil {
		t.Fatalf("ShouldDefer returned error: %v", err)
	}
	if decision.Defer {
		t.Errorf("expected Defer=false when policy allows battery and Low Power Mode, got reason: %s", decision.Reason)
	}
}

func TestShouldDeferRecoversOncePluggedIn(t *testing.T) {
	policy := PowerPolicy{RunOnBattery: false, RunInLowPowerMode: true}

	onBattery := &PowerPolicyService{
		run: func(ctx context.Context, name string, args ...string) (string, error) {
			if len(args) > 0 && args[len(args)-1] == "batt" {
				return samplePmsetBattBattery, nil
			}
			return samplePmsetGenLowPowerOff, nil
		},
	}
	decision, err := onBattery.ShouldDefer(context.Background(), policy)
	if err != nil {
		t.Fatalf("ShouldDefer returned error: %v", err)
	}
	if !decision.Defer {
		t.Fatalf("expected Defer=true while on battery")
	}

	pluggedIn := &PowerPolicyService{
		run: func(ctx context.Context, name string, args ...string) (string, error) {
			if len(args) > 0 && args[len(args)-1] == "batt" {
				return samplePmsetBattAC, nil
			}
			return samplePmsetGenLowPowerOff, nil
		},
	}
	decision, err = pluggedIn.ShouldDefer(context.Background(), policy)
	if err != nil {
		t.Fatalf("ShouldDefer returned error: %v", err)
	}
	if decision.Defer {
		t.Errorf("expected Defer=false once plugged into AC power, got reason: %s", decision.Reason)
	}
}
