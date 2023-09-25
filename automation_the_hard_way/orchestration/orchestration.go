package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/johnsiilver/gofordevopsclass/automation_the_hard_way/orchestration/config"
	"github.com/johnsiilver/gofordevopsclass/automation_the_hard_way/orchestration/lb/client"
	"github.com/johnsiilver/gofordevopsclass/automation_the_hard_way/orchestration/workflow"
	"github.com/rodaine/table"

	"github.com/fatih/color"

	pb "github.com/johnsiilver/gofordevopsclass/automation_the_hard_way/orchestration/lb/proto"
)

var (
	headerFmt = color.New(color.FgGreen, color.Underline).SprintfFunc()
	columnFmt = color.New(color.FgYellow).SprintfFunc()
)

func main() {
	flag.Parse()

	ctx := context.Background()

	wf, lb, err := setup()
	if err != nil {
		color.Red("Setup Error: %s", err)
		os.Exit(1)
	}

	// If the load balancer doesn't have pool "/", set one up.
	if _, err := lb.PoolHealth(ctx, "/", false, false); err != nil {
		err := lb.AddPool(
			ctx,
			"/",
			pb.PoolType_PT_P2C,
			client.HealthChecks{
				HealthChecks: []client.HealthCheck{
					client.StatusCheck{
						URLPath:       "/healthz",
						HealthyValues: []string{"ok", "OK"},
					},
				},
				Interval: 5 * time.Second,
			},
		)
		if err != nil {
			color.Red("LB did not have pool `/` and couldn't create it: %s", err)
			os.Exit(1)
		}
		color.Blue("Setup LB with pool `/`")
	}

	color.Red("Starting Workflow")
	if err := wf.Run(ctx); err != nil {
		status := wf.Status()
		color.Red("Workflow Failed: %s", status.EndState)

		var tbl table.Table
		if status.EndState == workflow.ESPreconditionFailure {
			tbl = table.New("Failed State", "Error")
			tbl.WithHeaderFormatter(headerFmt).WithFirstColumnFormatter(columnFmt)
			tbl.AddRow("Precondition", err)
		} else {
			tbl = table.New("Endpoint", "Failed State", "Error")
			tbl.WithHeaderFormatter(headerFmt).WithFirstColumnFormatter(columnFmt)
			for _, action := range status.Failures {
				tbl.AddRow(action.Endpoint, action.Failure(), action.Err())
			}
		}
		tbl.Print()
		os.Exit(1)
	}

	status := wf.Status()
	if len(status.Failures) == 0 {
		color.Blue("Workflow Completed with no failures")
		os.Exit(0)
	}

	color.Blue("Workflow Completed, but had %d failed actions", len(status.Failures))
	for i := 0; i < 3; i++ {
		color.Green("Retrying failed actions in 5 minutes...")
		time.Sleep(5 * time.Minute)
		fmt.Println("Executing failed actions...")

		wf.RetryFailed(ctx)
		status = wf.Status()
		if len(status.Failures) == 0 {
			break
		}
		color.Blue("Workflow Failures retry, but had %d failed actions", len(status.Failures))
	}
	status = wf.Status()
	if len(status.Failures) == 0 {
		color.Blue("Workflow Completed with no failures")
		os.Exit(0)
	}

	color.Blue("Workflow Completed but with %d failures after retries exhausted")
	tbl := table.New("Endpoint", "Failed State", "Error")
	tbl.WithHeaderFormatter(headerFmt).WithFirstColumnFormatter(columnFmt)

	for _, action := range status.Failures {
		failure := action.Failure()
		if failure == "" {
			failure = "during setup"
		}
		tbl.AddRow(action.Endpoint, failure, action.Err)
	}
	tbl.Print()
	os.Exit(0)
}

func setup() (*workflow.Workflow, *client.Client, error) {
	if len(flag.Args()) != 1 {
		return nil, nil, fmt.Errorf("must have argument to service file")
	}

	b, err := os.ReadFile(flag.Args()[0])
	if err != nil {
		return nil, nil, fmt.Errorf("can't open workflow configuration file: %w", err)
	}

	config := &config.Config{}
	if err := json.Unmarshal(b, config); err != nil {
		return nil, nil, fmt.Errorf("%q is misconfigured: %w", flag.Args()[0], err)
	}
	if err := config.Validate(); err != nil {
		log.Println(string(b))
		return nil, nil, fmt.Errorf("config file didn't validate: %w", err)
	}

	lb, err := client.New(config.LB)
	if err != nil {
		return nil, nil, fmt.Errorf("can't connected to LB(%s): %s", config.LB, err)
	}
	wf, err := workflow.New(config, lb)
	if err != nil {
		return nil, nil, fmt.Errorf("could not create workflow: %w", err)
	}
	return wf, lb, nil
}
