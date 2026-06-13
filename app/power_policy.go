package main

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// PowerStatus describes the machine's current power state.
type PowerStatus struct {
	OnBattery    bool
	LowPowerMode bool
}

// DeferDecision describes whether a scheduled scan or database update should
// be deferred given the current power state and the user's PowerPolicy.
type DeferDecision struct {
	Defer  bool
	Reason string
}

type powerCommandRunner func(ctx context.Context, name string, args ...string) (string, error)

// PowerPolicyService inspects the machine's power state via pmset and decides
// whether scheduled work should be deferred per PowerPolicy settings.
type PowerPolicyService struct {
	run powerCommandRunner
}

func newPowerPolicyService() *PowerPolicyService {
	return &PowerPolicyService{}
}

// ReadStatus returns the current battery / Low Power Mode state.
func (s *PowerPolicyService) ReadStatus(ctx context.Context) (PowerStatus, error) {
	run := s.runner()

	battOutput, err := run(ctx, "/usr/bin/pmset", "-g", "batt")
	if err != nil {
		return PowerStatus{}, fmt.Errorf("讀取電源狀態失敗: %w", err)
	}

	genOutput, err := run(ctx, "/usr/bin/pmset", "-g")
	if err != nil {
		return PowerStatus{}, fmt.Errorf("讀取電源設定失敗: %w", err)
	}

	return parsePowerStatus(battOutput, genOutput), nil
}

// ShouldDefer reports whether a scheduled scan or database update should be
// deferred given the current power state and the user's PowerPolicy. Callers
// should re-evaluate ShouldDefer on the next tick so that a deferred run
// proceeds once the machine is plugged in or Low Power Mode is turned off.
func (s *PowerPolicyService) ShouldDefer(ctx context.Context, policy PowerPolicy) (DeferDecision, error) {
	status, err := s.ReadStatus(ctx)
	if err != nil {
		return DeferDecision{}, err
	}

	if status.OnBattery && !policy.RunOnBattery {
		return DeferDecision{Defer: true, Reason: "使用電池電力時已設定不執行掃描或更新"}, nil
	}
	if status.LowPowerMode && !policy.RunInLowPowerMode {
		return DeferDecision{Defer: true, Reason: "低耗電模式時已設定不執行掃描或更新"}, nil
	}
	return DeferDecision{}, nil
}

func (s *PowerPolicyService) runner() powerCommandRunner {
	if s.run != nil {
		return s.run
	}
	return runPmset
}

func runPmset(ctx context.Context, name string, args ...string) (string, error) {
	output, err := exec.CommandContext(ctx, name, args...).Output()
	return string(output), err
}

// parsePowerStatus extracts the power source and Low Power Mode setting from
// `pmset -g batt` and `pmset -g` output respectively.
func parsePowerStatus(battOutput string, genOutput string) PowerStatus {
	status := PowerStatus{}

	for _, line := range strings.Split(battOutput, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "Now drawing from") {
			status.OnBattery = strings.Contains(trimmed, "Battery Power")
			break
		}
	}

	scanner := bufio.NewScanner(strings.NewReader(genOutput))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 2 && fields[0] == "lowpowermode" {
			status.LowPowerMode = fields[1] != "0"
			break
		}
	}

	return status
}
