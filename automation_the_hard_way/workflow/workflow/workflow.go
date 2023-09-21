package main

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fatih/color"
	"github.com/johnsiilver/gofordevopsclass/automation_the_hard_way/workflow/actions"
	"github.com/johnsiilver/gofordevopsclass/automation_the_hard_way/workflow/config"
	"github.com/johnsiilver/gofordevopsclass/automation_the_hard_way/workflow/lb/client"

	pb "github.com/johnsiilver/gofordevopsclass/automation_the_hard_way/workflow/lb/proto"
)

// EndStates are the final states after a run of a workflow.
//
//go:generate stringer -type=endState
type EndState int8

const (
	// ESUnknown indicates we haven't reached an end state.
	ESUnknown EndState = 0
	// ESSuccess means that the workflow has completed successfully. This
	// does not mean there haven't been failurea.
	ESSuccess EndState = 1
	// ESPreconditionFailure means no work was done as we failed on a precondition.
	ESPreconditionFailure EndState = 2
	// ESCanaryFailure indicates one of the canaries failed, stopping the workflow.
	ESCanaryFailure EndState = 3
	// ESMaxFailures indicates that the workflow passed the canary phase, but failed
	// at a later phase.
	ESMaxFailures EndState = 4
)

// Workflow represents our rollout Workflow.
type Workflow struct {
	config *config.Config
	lb     *client.Client

	failures int32
	endState EndState

	actions []*actions.Actions
}

// New creates a new workflow.
func New(config *config.Config, lb *client.Client) (*Workflow, error) {
	wf := &Workflow{
		config: config,
		lb:     lb,
	}
	if err := wf.buildActions(); err != nil {
		return nil, err
	}
	return wf, nil
}

// Runs runs our workflow on the supplied "actions" doing "canaryNum" canaries,
// then running "concurrency" number of actions that will stop at "maxFailures" number of
// failurea.
func (w *Workflow) Run(ctx context.Context) error {
	// Run a local precondition to make sure our load balancer is in a healthy state.
	preCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	err := w.checkLBState(preCtx)
	cancel()
	if err != nil {
		w.endState = ESPreconditionFailure
		return fmt.Errorf("checkLBState precondition fail: %s", err)
	}

	// Run our canaries one at a time. Any problem stops the workflow.
	for i := 0; i < len(w.actions) && int32(i) < w.config.CanaryNum; i++ {
		color.Green("Running canary on: %s", w.actions[i].Endpoint())
		ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
		err := w.actions[i].Run(ctx)
		cancel()
		if err != nil {
			w.endState = ESCanaryFailure
			return fmt.Errorf("canary failure on endpoint(%s): %w", w.actions[i].Endpoint(), err)
		}
		color.Yellow("Sleeping after canary for 1 minutes")
		time.Sleep(1 * time.Minute)
	}

	limit := make(chan struct{}, w.config.Concurrency)
	wg := sync.WaitGroup{}

	// Run the rest of the actions, with a limit to our concurrency.
	for i := w.config.CanaryNum; int(i) < len(w.actions); i++ {
		i := i
		limit <- struct{}{}
		if atomic.LoadInt32(&w.failures) > w.config.MaxFailures {
			break
		}
		wg.Add(1)
		go func() {
			defer func() { <-limit }()
			defer wg.Done()
			ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)

			color.Green("Upgrading endpoint: %s", w.actions[i].Endpoint())
			err := w.actions[i].Run(ctx)
			cancel()
			if err != nil {
				color.Red("Endpoint(%s) had upgrade error: %s", w.actions[i].Endpoint(), err)
				atomic.AddInt32(&w.failures, 1)
			}
		}()
	}
	wg.Wait()

	if atomic.LoadInt32(&w.failures) > w.config.MaxFailures {
		w.endState = ESMaxFailures
		return errors.New("exceeded max failures")
	}
	w.endState = ESSuccess
	return nil
}

// retryFailed retries all failed actiona. This is only used if
func (w *Workflow) retryFailed(ctx context.Context) {
	if w.endState != ESSuccess {
		panic("retrlyFailed cannot be called unless the workflow was a success")
	}

	ws := w.status()

	wg := sync.WaitGroup{}

	for i := 0; i < len(ws.failures); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)

			err := ws.failures[i].Run(ctx)
			cancel()
			if err == nil {
				atomic.AddInt32(&w.failures, -1)
			}
		}()
	}
	wg.Wait()
}

// checkLBState checks the load balancer pool for "pattern" contains all "endpoints"
// in a healthy state.
func (w *Workflow) checkLBState(ctx context.Context) error {
	ph, err := w.lb.PoolHealth(ctx, w.config.Pattern, true, true)
	if err != nil {
		return fmt.Errorf("PoolHealth(%s) error: %w", w.config.Pattern, err)
	}

	switch ph.Status {
	// If the pool is empty, make sure it should be.
	case pb.PoolStatus_PS_EMPTY:
		if len(w.config.Backends) != 0 {
			return fmt.Errorf("expected backends(%d) != found backends(%d)", len(w.config.Backends), len(ph.Backends))
		}
		return nil
	// We do some extra checks to make sure that while all the nodes in the pool are working,
	// we should have the backends that the config tells us are there.
	case pb.PoolStatus_PS_FULL:
		if len(w.config.Backends) != len(ph.Backends) {
			return fmt.Errorf("expected backends(%d) != found backends(%d)", len(w.config.Backends), len(ph.Backends))
		}
		m := map[string]bool{}
		for _, e := range w.config.Backends {
			m[e] = true
		}
		for _, hb := range ph.Backends {
			switch {
			case hb.Backend.GetIpBackend() != nil:
				b := hb.Backend.GetIpBackend()
				if !m[b.Ip] {
					return fmt.Errorf("configured backend %q not in config file", b.Ip)
				}
			default:
				return fmt.Errorf("we only support IPBackend, got %T", hb.Backend)
			}
		}
	default:
		return fmt.Errorf("pool was not at full health, was %s", ph.Status)
	}
	return nil
}

// buildActions builds actions from our configuration file.
func (w *Workflow) buildActions() error {
	for _, b := range w.config.Backends {
		a, err := actions.New(b, w.config, w.lb)
		if err != nil {
			return err
		}
		w.actions = append(w.actions, a)
	}
	return nil
}

type workflowStatus struct {
	// failures is a list of failed actiona.
	failures []*actions.Actions
	// endState is the endState of the workflow.
	endState EndState
}

// status will return the workflow's status after run() has completed
func (w *Workflow) status() workflowStatus {
	ws := workflowStatus{endState: w.endState}
	for _, a := range w.actions {
		if a.Err() != nil {
			ws.failures = append(ws.failures, a)
		}
	}
	return ws
}
