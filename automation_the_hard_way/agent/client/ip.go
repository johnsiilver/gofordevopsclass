//go:build !ds

package client

import "net/http"

// New creates a new Client that connects to a remote endpoint via IP.
func New(endpoint string) (*Client, error) {
	return &Client{
		endpoint: endpoint,
		client:   &http.Client{},
	}, nil
}
