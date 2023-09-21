package msgs

import "fmt"

// InstallReq is the request to install a package.
type InstallReq struct {
	// Name is the name of the package. It can contain no spaces and only
	// ASCII letters and numbers. This will be installed in the user's home
	// directory with the following path: ~/sa/<name>/.
	Name string
	// Package is the package directory to be installed at <name>. It is a
	// gzipped directory with our binary in it.
	Package []byte
	// Binary is the name of the binary to run. It must be directly in <name>.
	// Only ASCII letters and numbers are allowed.
	Binary string
	// Args are the arguments to pass to the binary.
	Args []string

	unzipped []byte
}

// Validate validates the InstallReq.
func (i *InstallReq) Validate() error {
	if i.Name == "" {
		return fmt.Errorf("name cannot be empty")
	}
	if i.Binary == "" {
		return fmt.Errorf("binary cannot be empty")
	}
	if len(i.Package) == 0 {
		return fmt.Errorf("package cannot be empty")
	}

	return nil
}

// InstallResp is the response to install a package.
type InstallResp struct {
	// ErrMsg is error message that was returned. If empty, no error occurred.
	ErrMsg string
}

// CPUPerfs is a list of CPU performance metrics.
type CPUPerfs struct {
	// ResolutionSecs is the number of seconds between each metric.
	ResolutionSecs int32
	// UnixTimeNano is the unix time in nanoseconds of the first metric.
	UnixTimeNano int64
	// CPU is the list of CPU metrics. On linux this will be the per CPU metrics.
	// On Darwin, this will be the total CPU metric, since that is all we can get
	// without wrapping some C calls to get the per CPU metrics.
	CPU []CPUPerf
}

// CPUPerf is the CPU performance metrics for either the entire system or all CPUs.
type CPUPerf struct {
	// ID is the ID of the CPU. This is not provided on Darwin.
	ID string
	// User is the amount of time the CPU is running user code.
	User float64
	// System is the amount of time the CPU is running kernel code.
	System float64
	// Idle is the amount of time the CPU is idle.
	Idle float64
	// IOWait is the amount of time the CPU is waiting for IO to complete. This is
	// not provided on Darwin.
	IOWait float64
	// IRQ is the amount of time the CPU is waiting for IRQ to complete. This is
	// not provided on Darwin.
	IRQ float64
}

type MemPerf struct {
	ResolutionSecs int32
	UnixTimeNano   int64
	// Total is the total amount of physical memory on the machine.
	Total uint64
	// Free is the amount of physical memory that is free.
	Free uint64
	// Avail is the amount of memory that can be used without causing swapping.
	// This is not the amount of memory that is free.
	// This is not provided on Darwin, becuase I am not sure how to calculate it.
	Avail uint64
}
