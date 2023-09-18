package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

var n = flag.Int("n", 10, "number of iterations")

func main() {
	flag.Parse()

	if *n <= 0 {
		fmt.Println("n must be greater than 0")
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	wg := &sync.WaitGroup{}
	exitLock := &sync.Mutex{}

	signals := make(chan os.Signal, 1)
	signal.Notify(
		signals,
		syscall.SIGHUP,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGQUIT,
	)

	go func() {
		for {
			switch sig := <-signals; sig {
			case syscall.SIGHUP: // Terminal closed
			// Do nothing, let it keep going
			case syscall.SIGINT, syscall.SIGTERM: // User hit Ctrl+C or told to quit via another program
				go func() {
					log.Printf("%s called", sig.String())
					exitLock.Lock()

					cancel()
					wg.Wait()
					os.Exit(1)
				}()
			case syscall.SIGQUIT: // User hit Ctrl+\
				go func() {
					log.Println("SIGQUIT called")
					exitLock.Lock()

					cancel()
					wg.Wait()
					panic("SIGQUIT called") // Gives us a core dump
				}()
			default:
				log.Println(sig)
			}
		}
	}()

	wg.Add(1)
	func() {
		defer wg.Done()
		for i := 0; i < *n; i++ {
			fmt.Println(i)
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
			}
		}
	}()
	exitLock.Lock() // If there is a signal pending, wait for it to finish

	if ctx.Err() != nil {
		log.Println(ctx.Err())
		os.Exit(1)
	}
}
