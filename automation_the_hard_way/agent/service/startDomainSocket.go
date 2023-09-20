//go:build ds

package service

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
)

// Start starts the agent on a domain socket instead of an IP. To use this method,
// you must compile with "go build -tags ds".
func (a *Agent) Start() error {
	var sockAddr = filepath.Join(a.homePath, "/sa/socket/sa.sock")
	if err := os.MkdirAll(filepath.Dir(sockAddr), 0700); err != nil {
		return fmt.Errorf("could not create socket dir path: %w", err)
	}
	// Remove old socket file if it exists.
	os.Remove(sockAddr)

	l, err := net.Listen("unix", sockAddr)
	if err != nil {
		return fmt.Errorf("could not connect to socket: %w", err)
	}

	a.router.Run(a.addr)

	return a.router.RunListener(l)
}
