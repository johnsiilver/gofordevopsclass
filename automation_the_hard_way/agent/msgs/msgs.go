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
	CPU          []CPUPerf
}

type CPUPerf struct {
	ID     string
	User   int32
	System int32
	Idle   int32
	IOWait int32
	IRQ    int32
}

type MemPerf struct {
	ResolutionSecs int32
	UnixTimeNano   int64
	Total          int32
	Free           int32
	Avail          int32
}
