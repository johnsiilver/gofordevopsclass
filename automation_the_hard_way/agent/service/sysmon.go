package service

import (
	"context"
	"expvar"
	"fmt"
	"log"
	"sync"
	"time"
)

const resolutionSecs = 10

func (a *Agent) perfLoop() error {
	const resolutionSecs = 10

	ctx, cancel := context.WithTimeout(context.Background(), resolutionSecs*time.Second)
	defer cancel()

	if err := a.collectCPU(ctx, resolutionSecs); err != nil {
		return fmt.Errorf("unable to collect CPU data: %s", err)
	}
	if err := a.collectMem(ctx, resolutionSecs); err != nil {
		return fmt.Errorf("unable to collect memory data: %s", err)
	}

	expvar.Publish(
		"system-cpu",
		expvar.Func(
			func() interface{} {
				return a.cpuData.Load()
			},
		),
	)
	expvar.Publish(
		"system-mem",
		expvar.Func(
			func() interface{} {
				return a.memData.Load()
			},
		),
	)

	go func() {
		wg := sync.WaitGroup{}
		for {
			time.Sleep(resolutionSecs * time.Second)

			ctx, cancel = context.WithTimeout(context.Background(), resolutionSecs*time.Second)
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := a.collectCPU(ctx, resolutionSecs); err != nil {
					log.Println(err)
				}
			}()
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := a.collectMem(ctx, resolutionSecs); err != nil {
					log.Println(err)
				}
			}()
			wg.Wait()
			cancel()
		}
	}()
	return nil
}
