package config

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

// Config represents the configuration file that details the work to be done.
type Config struct {
	// Concurrency is the number of servers that can be upgraded at a time.
	Concurrency int32
	// CanaryNum is the number of canaries to do before proceeding with a general rollout.
	// Any canary failure fails the workflow. Canaries execute one at a time.
	CanaryNum int32
	// MaxFailures is the maximum number of failures to tolerate before stopping.
	// You can have more failures than MaxFailures due to concurrency settings.
	MaxFailures int32
	// Src is the path on disk to the binary to push.
	Src string
	// LB is the host:port of the load balancer.
	LB string
	// Pattern is the load balancer's Pool pattern.
	Pattern string
	// Backends are the backends that need to be updated, simply the host in IP:Port form.
	Backends []string
}

// Validate does basic validation of the config.
func (s Config) Validate() error {
	if _, _, err := CheckIPPort(s.LB); err != nil {
		return fmt.Errorf("LB(%s) is not correct: %w", s.LB, err)
	}
	if len(s.Backends) < 1 {
		return fmt.Errorf("must specify some Backends")
	}
	for _, b := range s.Backends {
		_, _, err := CheckIPPort(b)
		if err != nil {
			return fmt.Errorf("Backend(%s) is not correct: %w", b, err)
		}
	}
	if strings.TrimSpace(s.Pattern) == "" {
		return fmt.Errorf("Pattern(%s) is invalid", s.Pattern)
	}
	if s.Concurrency < 1 {
		return fmt.Errorf("Concurrency(%d) is invalid", s.Concurrency)
	}
	return nil
}

// CheckIPPort takes a host:port string and splits it out and verifies it.
func CheckIPPort(b string) (net.IP, int32, error) {
	h, ps, err := net.SplitHostPort(b)
	if err != nil {
		return nil, 0, err
	}
	p, _ := strconv.Atoi(ps)
	ip := net.ParseIP(h)
	if ip == nil {
		return nil, 0, err
	}
	if p < 1 || p > 65534 {
		return nil, 0, fmt.Errorf("invalid port: %d", p)
	}
	return ip, int32(p), nil
}
