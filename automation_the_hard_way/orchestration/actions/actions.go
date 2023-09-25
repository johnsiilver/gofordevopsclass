package actions

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/johnsiilver/gofordevopsclass/automation_the_hard_way/orchestration/config"
	"github.com/johnsiilver/gofordevopsclass/automation_the_hard_way/orchestration/lb/client"
)

// StateFn is a function that represents a state in the workflow.
type StateFn func(ctx context.Context) (StateFn, error)

// Actions is the set of actions to take on a single endpoint, one at a time.
type Actions struct {
	// endpoint is the machine to connect to.
	endpoint string
	// backend is the backend configuraiton in the load balancer.
	// This is used to remove and add the backend to the load balancer.
	backend client.IPBackend
	// config is the configuration for the workflow.
	config *config.Config
	// srcf is the file to copy to the remote machine.
	srcf *os.File
	// dst is the destination path on the remote machine to copy the file to.
	dst string
	// lb is the load balancer client.
	lb *client.Client

	// started indicates if the workflow has started.
	started bool
	// failedState is the state to start at if we have a failure.
	failedState StateFn
	// err is the error that caused the failure.
	err error
}

// New creates a new Actions.
func New(endpoint string, cfg *config.Config, lb *client.Client) (*Actions, error) {
	ip, port, err := config.CheckIPPort(endpoint)
	if err != nil {
		return nil, err
	}

	return &Actions{
		endpoint: endpoint,
		backend:  client.IPBackend{IP: ip, Port: port},
		config:   cfg,
		lb:       lb,
	}, nil
}

// Endpoint returns the endpoint this Actions is for.
func (a *Actions) Endpoint() string {
	return a.endpoint
}

// Err returns the error that caused the failure.
func (a *Actions) Err() error {
	return a.err
}

// Run runs the workflow.
func (a *Actions) Run(ctx context.Context) (err error) {
	a.srcf, err = os.Open(a.config.Src)
	if err != nil {
		a.err = fmt.Errorf("cannot open binary to copy(%s): %w", a.config.Src, err)
		return a.err
	}

	fn := a.findAppLocal
	if a.failedState != nil {
		fn = a.failedState
	}

	a.started = true
	for {
		if ctx.Err() != nil {
			a.err = ctx.Err()
			return ctx.Err()
		}
		fn, err = fn(ctx)
		if err != nil {
			a.failedState = fn
			a.err = err
			return err
		}
		if fn == nil {
			return nil
		}
	}
}

// findAppLocal finds the app on the local machine by querying the /installedAt URL.
// In real life, you would do this in a more robust way, but this is just a demo.
// The orignal version of this has a hardcoded path to the binary and we did this with SFTP.
// However, to avoid all the SSH setup and do this all on your local machine, we are using
// a little bandaid here.
func (a *Actions) findAppLocal(ctx context.Context) (StateFn, error) {
	c := &http.Client{}

	u := &url.URL{
		Host:   a.endpoint,
		Path:   "/installedAt",
		Scheme: "http",
	}

	req, err := http.NewRequestWithContext(
		ctx,
		"GET",
		u.String(),
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("problem creating HTTP request: %w", err)
	}

	resp, err := c.Do(req)
	if err != nil {
		time.Sleep(1 * time.Second)
		return nil, errors.New("findAppLocal() timed out")
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("problem reading response body: %w", err)
	}
	if len(b) == 0 {
		return nil, errors.New("findAppLocal() returned nothing")
	}

	a.dst = strings.TrimSpace(string(b))
	a.dst = filepath.Clean(a.dst)
	return a.rmBackend, nil
}

// rmBackend removes the backend from the load balancer.
func (a *Actions) rmBackend(ctx context.Context) (StateFn, error) {
	err := a.lb.RemoveBackend(ctx, a.config.Pattern, a.backend)
	if err != nil {
		return nil, fmt.Errorf("problem removing backend from pool: %w", err)
	}

	return a.jobKill, nil
}

const (
	SIGTERM = 15
	SIGKILL = 9
)

// jobKill kills the existing job on the remote machine.
func (a *Actions) jobKill(ctx context.Context) (StateFn, error) {
	pid, err := a.findPID(ctx)
	if err != nil {
		return nil, fmt.Errorf("problem finding existing PIDs: %w", err)
	}

	if pid == "" {
		return nil, fmt.Errorf("could not locate a job for backend: %s", a.endpoint)
	}

	if err := a.killPID(ctx, pid, SIGTERM); err != nil {
		return nil, fmt.Errorf("failed to kill existing PIDs: %w", err)
	}

	if err := a.waitForDeath(ctx, pid, 30*time.Second); err != nil {
		if err := a.killPID(ctx, pid, SIGKILL); err != nil {
			return nil, fmt.Errorf("failed to kill existing PIDs: %w", err)
		}
		if err := a.waitForDeath(ctx, pid, 10*time.Second); err != nil {
			return nil, fmt.Errorf("failed to kill existing PIDs after -9: %w", err)
		}
		return a.cp, nil
	}
	return a.cp, nil
}

// cp copies the binary to the remote machine.
func (a *Actions) cp(ctx context.Context) (StateFn, error) {
	dstf, err := os.OpenFile(a.dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0770)
	if err != nil {
		return nil, fmt.Errorf("could not open new file on remote destination(%s): %w", a.dst, err)
	}
	defer dstf.Close()

	_, err = io.Copy(dstf, a.srcf)
	if err != nil {
		return nil, fmt.Errorf("SFTP failed to do a complete copy: %w", err)
	}

	return a.jobStart, nil
}

// jobStart starts the binary on the remote machine.
func (a *Actions) jobStart(ctx context.Context) (StateFn, error) {
	if err := a.runBinary(ctx); err != nil {
		return nil, fmt.Errorf("failed to start binary after copy: %w", err)
	}
	return a.reachable(ctx)
}

// reachable waits for the binary to be reachable on /healthz.
func (a *Actions) reachable(ctx context.Context) (StateFn, error) {
	ctx, cancel := context.WithTimeout(ctx, 1*time.Minute)
	defer cancel()

	c := &http.Client{}

	u := &url.URL{
		Host:   a.endpoint,
		Path:   "/healthz",
		Scheme: "http",
	}

	req, err := http.NewRequestWithContext(
		ctx,
		"GET",
		u.String(),
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("problem creating HTTP request: %w", err)
	}

	for {
		if ctx.Err() != nil {
			return nil, errors.New("reachable() timed out")
		}

		resp, err := c.Do(req)
		if err != nil {
			time.Sleep(1 * time.Second)
			continue
		}
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			continue
		}

		if strings.TrimSpace(string(b)) == "ok" {
			return a.addBackend, nil
		}
	}
}

// addBackend adds the backend to the load balancer.
func (a *Actions) addBackend(ctx context.Context) (StateFn, error) {
	err := a.lb.AddBackend(ctx, a.config.Pattern, a.backend)
	if err != nil {
		return nil, err
	}

	return nil, nil
}

// findPID finds the PIDs of the running binary.
func (a *Actions) findPID(ctx context.Context) (string, error) {
	serviceName := path.Base(a.dst)
	cmdStr := fmt.Sprintf(`ps -A | grep %s | grep -v grep | awk '{print $1}'`, serviceName)
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", cmdStr)
	result, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}

	pid := strings.TrimSpace(string(result))
	if strings.Contains(pid, "\n") {
		return "", fmt.Errorf("found more than one pid with name %q", a.dst)
	}
	return pid, nil
}

// killPID sends the PIDs the given signal.
func (a *Actions) killPID(ctx context.Context, pid string, signal syscall.Signal) error {
	switch signal {
	case SIGTERM, SIGKILL:
		// Do nothing
	default:
		return fmt.Errorf("sent killPID a non-termination signal: %d", signal)
	}
	cmd := exec.CommandContext(ctx, "kill", "-"+strconv.Itoa(int(signal)), pid)
	if _, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("problem kiling pid %s: %w", pid, err)
	}
	return nil
}

// waitForDeath waits for the pid to die or times out.
func (a *Actions) waitForDeath(ctx context.Context, pid string, timeout time.Duration) error {
	t := time.NewTimer(timeout)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			return errors.New("timeout waiting for pids death")
		default:
		}

		pid, err := a.findPID(ctx)
		if err != nil {
			return fmt.Errorf("findPIDs giving errors: %w", err)
		}
		if pid == "" {
			return nil
		}

		time.Sleep(1 * time.Second)
	}
}

// runBinary runs the binary on the remote machine.
func (a *Actions) runBinary(ctx context.Context) error {
	cmdStr := fmt.Sprintf("nohup %s -port %d &", a.dst, a.backend.Port)
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", cmdStr)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("problem running the binary on the remote side: %w", err)
	}

	return nil
}

func (a *Actions) Failure() string {
	if a.failedState == nil {
		return ""
	}

	return runtime.FuncForPC(reflect.ValueOf(a.failedState).Pointer()).Name()
}
