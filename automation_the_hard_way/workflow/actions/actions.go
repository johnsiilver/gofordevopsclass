package actions

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/johnsiilver/gofordevopsclass/automation_the_hard_way/workflow/config"
	"github.com/johnsiilver/gofordevopsclass/automation_the_hard_way/workflow/lb/client"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
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
	// sshClient is the SSH client to the remote machine.
	sshClient *ssh.Client

	// started indicates if the workflow has started.
	started bool
	// failedState is the state to start at if we have a failure.
	failedState StateFn
	// err is the error that caused the failure.
	err error
}

// New creates a new Actions.
func New(endpoint string, cfg *config.Config, lb *client.Client) (*Actions, error) {
	ip, err := config.CheckIP(endpoint)
	if err != nil {
		return nil, err
	}
	return &Actions{
		endpoint: endpoint,
		backend:  client.IPBackend{IP: ip, Port: int32(cfg.BinaryPort)},
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

func (a *Actions) Run(ctx context.Context) (err error) {
	a.srcf, err = os.Open(a.config.Src)
	if err != nil {
		a.err = fmt.Errorf("cannot open binary to copy(%s): %w", a.config.Src, err)
		return a.err
	}

	back := a.endpoint + ":22"
	a.sshClient, err = ssh.Dial("tcp", back, a.config.SSH)
	if err != nil {
		a.err = fmt.Errorf("problem dialing the endpoint(%s): %w", back, err)
		return a.err
	}
	defer a.sshClient.Close()

	fn := a.rmBackend
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

func (a *Actions) rmBackend(ctx context.Context) (StateFn, error) {
	err := a.lb.RemoveBackend(ctx, a.config.Pattern, a.backend)
	if err != nil {
		return nil, fmt.Errorf("problem removing backend from pool: %w", err)
	}

	return a.jobKill, nil
}

func (a *Actions) jobKill(ctx context.Context) (StateFn, error) {
	pids, err := a.findPIDs(ctx)
	if err != nil {
		return nil, fmt.Errorf("problem finding existing PIDs: %w", err)
	}

	if len(pids) == 0 {
		return a.cp, nil
	}

	if err := a.killPIDs(ctx, pids, 15); err != nil {
		return nil, fmt.Errorf("failed to kill existing PIDs: %w", err)
	}

	if err := a.waitForDeath(ctx, pids, 30*time.Second); err != nil {
		if err := a.killPIDs(ctx, pids, 9); err != nil {
			return nil, fmt.Errorf("failed to kill existing PIDs: %w", err)
		}
		if err := a.waitForDeath(ctx, pids, 10*time.Second); err != nil {
			return nil, fmt.Errorf("failed to kill existing PIDs after -9: %w", err)
		}
		return a.cp, nil
	}
	return a.cp, nil
}

func (a *Actions) cp(ctx context.Context) (StateFn, error) {
	if err := a.sftp(); err != nil {
		return nil, fmt.Errorf("failed to cp binary to remote end: %w", err)
	}
	return a.jobStart, nil
}

func (a *Actions) jobStart(ctx context.Context) (StateFn, error) {
	if err := a.runBinary(ctx); err != nil {
		return nil, fmt.Errorf("failed to start binary after copy: %w", err)
	}
	return a.reachable(ctx)
}

func (a *Actions) reachable(ctx context.Context) (StateFn, error) {
	ctx, cancel := context.WithTimeout(ctx, 1*time.Minute)
	defer cancel()

	c := &http.Client{}

	u := &url.URL{
		Host:   net.JoinHostPort(a.endpoint, strconv.Itoa(a.config.BinaryPort)),
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

func (a *Actions) addBackend(ctx context.Context) (StateFn, error) {
	err := a.lb.AddBackend(ctx, a.config.Pattern, a.backend)
	if err != nil {
		return nil, err
	}

	return nil, nil
}

func (a *Actions) findPIDs(ctx context.Context) ([]string, error) {
	serviceName := path.Base(a.config.Src)

	result, err := a.combinedOutput(
		ctx,
		a.sshClient,
		fmt.Sprintf("pidof %s", serviceName),
	)
	if err != nil {
		if err.(*ssh.ExitError).ExitStatus() == 127 {
			return nil, err
		}
		return nil, nil
	}

	return strings.Split(strings.TrimSpace(result), " "), nil
}

func (a *Actions) killPIDs(ctx context.Context, pids []string, signal syscall.Signal) error {
	for _, pid := range pids {
		_, err := a.combinedOutput(
			ctx,
			a.sshClient,
			fmt.Sprintf("kill -s %d %s", signal, pid),
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func (a *Actions) waitForDeath(ctx context.Context, pids []string, timeout time.Duration) error {
	t := time.NewTimer(timeout)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			return errors.New("timeout waiting for pids death")
		default:
		}

		results, err := a.findPIDs(ctx)
		if err != nil {
			return fmt.Errorf("findPIDs giving errors: %w", err)
		}

		if len(results) == 0 {
			return nil
		}
		time.Sleep(1 * time.Second)
	}
}

func (a *Actions) runBinary(ctx context.Context) error {
	err := a.startOnly(
		ctx,
		a.sshClient,
		fmt.Sprintf("/usr/bin/nohup %s &", a.config.Dst),
	)
	if err != nil {
		return fmt.Errorf("problem running the binary on the remove side: %w", err)
	}
	return nil
}

func (a *Actions) sftp() error {
	c, err := sftp.NewClient(a.sshClient)
	if err != nil {
		return fmt.Errorf("could not create SFTP client: %w", err)
	}
	defer c.Close()

	dstf, err := c.OpenFile(a.config.Dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	if err != nil {
		return fmt.Errorf("SFTP could not open file on remote destination(%s): %w", a.config.Dst, err)
	}
	defer dstf.Close()
	if err := dstf.Chmod(0770); err != nil {
		return fmt.Errorf("SFTP could not set the file mode to 0770: %w", err)
	}

	_, err = io.Copy(dstf, a.srcf)
	if err != nil {
		return fmt.Errorf("SFTP failed to do a complete copy: %w", err)
	}
	return nil
}

// combinedOutput runs a command on an SSH client. The context can be cancelled, however
// SSH does not always honor the kill signals we send, so this might not break. So closing
// the session does nothing. So depending on what the server is doing, cancelling the context
// may do nothing and it may still block.
func (*Actions) combinedOutput(ctx context.Context, conn *ssh.Client, cmd string) (string, error) {
	sess, err := conn.NewSession()
	if err != nil {
		return "", err
	}
	defer sess.Close()

	if v, ok := ctx.Deadline(); ok {
		t := time.NewTimer(v.Sub(time.Now()))
		defer t.Stop()

		go func() {
			x := <-t.C
			if !x.IsZero() {
				sess.Signal(ssh.SIGKILL)
			}
		}()
	}

	b, err := sess.Output(cmd)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (*Actions) startOnly(ctx context.Context, conn *ssh.Client, cmd string) error {
	sess, err := conn.NewSession()
	if err != nil {
		return fmt.Errorf("could not start new SSH session: %w", err)
	}
	// Note: don't close the session, it will prevent the program from starting.

	return sess.Start(cmd)
}

func (a *Actions) failure() string {
	if a.failedState == nil {
		return ""
	}

	return runtime.FuncForPC(reflect.ValueOf(a.failedState).Pointer()).Name()
}
