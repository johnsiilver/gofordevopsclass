package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/cenk/backoff"
)

// HTTP is a wrapper around http.Client that implements an Exponential backoof pattern for
// HTTP requests.
type HTTP struct {
	client *http.Client
}

// New creates an new HTTP instance.
func New(client *http.Client) *HTTP {
	return &HTTP{
		client: client,
	}
}

// Do executes an HTTP request with an exponential backoff.
func (h *HTTP) Do(req *http.Request) (*http.Response, error) {
	if _, ok := req.Context().Deadline(); !ok {
		return nil, fmt.Errorf("all requests must have a Context deadline set")
	}
	var resp *http.Response

	op := func() error {
		var err error
		resp, err = h.client.Do(req)
		if err != nil {
			log.Println("error: unable to fetch URL: ", err)
			return err
		}
		if resp.StatusCode != 200 {
			return fmt.Errorf("non-200 response code")
		}
		return nil
	}

	err := backoff.Retry(
		op,
		&backoff.ExponentialBackOff{
			InitialInterval:     2 * time.Second,
			RandomizationFactor: 0.5,
			Multiplier:          2,
			MaxInterval:         10 * time.Second,
			Clock:               backoff.SystemClock,
		},
	)
	if err != nil {
		return nil, err
	}

	return resp, nil
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

		resp, err := h.Do(req)
		if err != nil {
			cancel()
			fmt.Println("error: unable to fetch URL: ", err)
			continue
		}
		resp.Body.Close()

		cancel()
		fmt.Println("success")
		time.Sleep(500 * time.Millisecond)
	}
}
