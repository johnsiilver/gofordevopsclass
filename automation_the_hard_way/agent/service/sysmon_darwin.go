//go:build darwin

package service

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"github.com/johnsiilver/gofordevopsclass/automation_the_hard_way/agent/msgs"
)

// totalMemory is the total memory on OSX. We get this from sysctl.
// As it shouldn't change during the life of the program, we can set it once.
var totalMemory int

func init() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// ex output: hw.memsize: 68719476736
	cmd := exec.CommandContext(ctx, "sysctl", "-a", "hw.memsize")
	b, err := cmd.Output()
	if err != nil {
		panic(fmt.Errorf("problem running the 'sysctl -a hw.memsize' command on darwin: %s", err))
	}

	sp := strings.Split(string(b), ":")
	if len(sp) != 2 {
		panic("sysctl -a hw.memsize output is not in expected format")
	}

	totalMemory, err = strconv.Atoi(strings.TrimSpace(sp[1]))
	if err != nil {
		panic(fmt.Errorf("sysctl -a hw.memsize output is not an integer: %s", sp[1]))
	}
}

func (a *Agent) collectCPU(ctx context.Context, resolutiona int32) error {
	p, err := getCPUPerf(ctx)
	if err != nil {
		return fmt.Errorf("problem getting CPU performance: %w", err)
	}

	v := &msgs.CPUPerfs{
		ResolutionSecs: resolutionSecs,
		UnixTimeNano:   time.Now().UnixNano(),
		CPU:            []msgs.CPUPerf{p},
	}
	a.cpuData.Store(v)
	return nil
}

func (a *Agent) collectMem(ctx context.Context, resolution int32) error {
	cmd := exec.CommandContext(ctx, "vm_stat")

	b, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("problem running the vm_stat command on darwin: %w", err)
	}

	stats, err := vmStatToMap(ctx, b)
	if err != nil {
		return fmt.Errorf("problem parsing vm_stat output on darwin: %w", err)
	}
	msg, err := calcPerf(ctx, stats)
	if err != nil {
		return fmt.Errorf("problem calculating memory performance on darwin: %w", err)
	}

	a.memData.Store(&msg)
	return nil
}

func getCPUPerf(ctx context.Context) (msgs.CPUPerf, error) {
	cmd := exec.CommandContext(ctx, "top", "-l", "1", "-n", "0")
	output, err := cmd.Output()
	if err != nil {
		return msgs.CPUPerf{}, err
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "CPU usage:") {
			re := regexp.MustCompile(`(\d+\.\d+)% user, (\d+\.\d+)% sys, (\d+\.\d+)% idle`)
			match := re.FindStringSubmatch(line)
			if len(match) == 4 {
				user, _ := strconv.ParseFloat(match[1], 64)
				sys, _ := strconv.ParseFloat(match[2], 64)
				idle, _ := strconv.ParseFloat(match[3], 64)
				return msgs.CPUPerf{
					ID:     "CPU",
					User:   user,
					System: sys,
					Idle:   idle,
				}, nil
			}
		}
	}

	return msgs.CPUPerf{}, fmt.Errorf("could not find CPU stats in top output")
}

/* vm_stat output looks like this:

Mach Virtual Memory Statistics: (page size of 16384 bytes)
Pages free:                               96637.
Pages active:                           1928038.
Pages inactive:                         1883545.
Pages speculative:                        20295.
Pages throttled:                              0.
Pages wired down:                        206471.
Pages purgeable:                          44535.
"Translation faults":                 143114484.
Pages copy-on-write:                    5867292.
Pages zero filled:                     78859343.
Pages reactivated:                       305797.
Pages purged:                            719667.
File-backed pages:                       759784.
Anonymous pages:                        3072094.
Pages stored in compressor:                7144.
Pages occupied by compressor:              2740.
Decompressions:                            4599.
Compressions:                             11798.
Pageins:                                2832780.
Pageouts:                                  1098.
Swapins:                                      0.
Swapouts:                                     0.
*/

var pageSizeRE = regexp.MustCompile(`page size of (\d+) bytes`)

func vmStatToMap(ctx context.Context, output []byte) (map[string]int, error) {
	const memTotalPrefix = "Mach Virtual Memory Statistics"

	if len(output) == 0 {
		return nil, fmt.Errorf("vm_stat output is empty")
	}
	m := make(map[string]int)

	for _, b := range bytes.Split(output, []byte("\n")) {
		b = bytes.TrimSpace(b)
		if len(b) == 0 {
			continue
		}
		s := unsafe.String(&b[0], len(b))

		// Special case:
		if strings.HasPrefix(s, memTotalPrefix) {
			matches := pageSizeRE.FindStringSubmatch(s)
			if len(matches) != 2 {
				return nil, fmt.Errorf("vm_stat output line(Mach Virtual Memory Statistics) is not in expected format: %s", string(b))
			}
			pageSize, err := strconv.Atoi(matches[1])
			if err != nil {
				return nil, fmt.Errorf("vm_stat output line(Mach Virtual Memory Statistics) is not an integer: %s", string(b))
			}
			m["Page size"] = pageSize
			continue
		}

		sp := strings.Split(s, ":")
		if len(sp) != 2 {
			log.Println(sp)
			return nil, fmt.Errorf("vm_stat output is not in expected format: %s", string(b))
		}
		attr := strings.TrimSpace(sp[0])

		numStr := strings.TrimSuffix(sp[1], ".")
		num, err := strconv.Atoi(strings.TrimSpace(numStr))
		if err != nil {
			return nil, fmt.Errorf("vm_stat has non-numeric attribute(%s): %s", attr, sp[1])
		}
		m[attr] = num
	}
	return m, nil
}

// calcPerf calculates the memory performance from the vm_stat output generated by vmStatToMap().
func calcPerf(ctx context.Context, vmStats map[string]int) (msgs.MemPerf, error) {
	switch {
	case vmStats["Pages free"] == 0:
		return msgs.MemPerf{}, fmt.Errorf("vm_stat 'Pages free' was not provided")
	case vmStats["Page size"] == 0:
		return msgs.MemPerf{}, fmt.Errorf("vm_stat 'Page size' was not provided")
	}

	return msgs.MemPerf{
		ResolutionSecs: resolutionSecs,
		UnixTimeNano:   time.Now().UnixNano(),
		Free:           uint64(vmStats["Pages free"] * vmStats["Page size"]),
		Total:          uint64(totalMemory),
	}, nil
}
