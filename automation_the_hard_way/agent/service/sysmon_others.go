//go:build !darwin

package service

import (
	"context"
	"time"

	"github.com/johnsiilver/gofordevopsclass/automation_the_hard_way/agent/msgs"

	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/mem"
)

func (a *Agent) collectCPU(ctx context.Context, resolutiona int32) error {
	stats, err := cpu.TimesWithContext(ctx, true)
	if err != nil {
		return err
	}

	v := &msgs.CPUPerfs{
		ResolutionSecs: resolutionSecs,
		UnixTimeNano:   time.Now().UnixNano(),
	}

	for _, stat := range stats {
		c := msgs.CPUPerf{
			ID:     stat.CPU,
			User:   stat.User,
			System: stat.System,
			Idle:   stat.Idle,
			IOWait: stat.Iowait,
			IRQ:    stat.Irq,
		}
		v.CPU = append(v.CPU, c)
	}
	a.cpuData.Store(v)
	return nil
}

func (a *Agent) collectMem(ctx context.Context, resolution int32) error {
	stats, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		return err
	}

	v := &msgs.MemPerf{
		ResolutionSecs: resolution,
		UnixTimeNano:   time.Now().UnixNano(),
		Total:          stats.Total,
		Free:           stats.Free,
		Avail:          stats.Available,
	}

	a.memData.Store(v)
	return nil
}
