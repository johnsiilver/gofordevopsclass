package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/sony/gobreaker"
)

// HTTP is a wrapper around http.Client that implements the CircuitBreaker pattern for Get requests.
type HTTP struct {
	client *http.Client
	cb     *gobreaker.CircuitBreaker
}

// New creates an new HTTP instance.
func New(client *http.Client) *HTTP {
	return &HTTP{
		client: client,
		cb: gobreaker.NewCircuitBreaker(
			gobreaker.Settings{
				MaxRequests: 1,                // only one request at a time if in the half-open state
				Interval:    30 * time.Second, // how long before we can leave the Half-Open state
				Timeout:     10 * time.Second, // how long to wait in Open before transiting to Half-Open
				ReadyToTrip: func(c gobreaker.Counts) bool {
					return c.ConsecutiveFailures > 5 // after 5 failures, trip the circuit
				},
			},
		),
	}
}

// Do executes an HTTP request.
func (h *HTTP) Do(req *http.Request) (*http.Response, error) {
	if _, ok := req.Context().Deadline(); !ok {
		return nil, fmt.Errorf("all requests must have a Context deadline set")
	}

	r, err := h.cb.Execute(
		func() (any, error) {
			resp, err := h.client.Do(req)
			if err != nil {
				return nil, err
			}
			if resp.StatusCode != 200 {
				return nil, fmt.Errorf("non-200 response code")
			}
			return resp, err
		},
	)
	if err != nil {
		return nil, err
	}
	return r.(*http.Response), nil
}

func main() {
	if len(os.Args) != 2 {
		fmt.Println("error: only one argument allowed, the URL to fetch")
		os.Exit(1)
	}

	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	h := New(client)

	for {
		req, err := http.NewRequest("GET", os.Args[1], nil)
		if err != nil {
			fmt.Println("error: unable to create request: ", err)
			os.Exit(1)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		req = req.WithContext(ctx)

		_, err = h.Do(req)
		if err != nil {
			cancel()
			fmt.Println("error: unable to fetch URL: ", err)
			time.Sleep(500 * time.Millisecond)
			continue
		}
		cancel()
		fmt.Println("success")
		time.Sleep(500 * time.Millisecond)
	}
}
