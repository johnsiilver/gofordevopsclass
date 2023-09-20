/*
Package client provides a client to the system agent that uses SSH and unix sockets
to make the connection.

The SSH forwarding is based on code from:
https://stackoverflow.com/questions/21417223/simple-ssh-port-forward-in-golang
*/
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"

	"github.com/johnsiilver/serveonssh"

	"github.com/johnsiilver/gofordevopsclass/automation_the_hard_way/agent/msgs"
)

// Client provides a client to the system agent that uses SSH and unix sockets
type Client struct {
	user     string
	endpoint string
	client   *http.Client
	p        serveonssh.Proxy
}

// Close closes the client.
func (c *Client) Close() error {
	c.client.CloseIdleConnections()
	if !reflect.ValueOf(c.p).IsZero() {
		c.p.Close()
	}
	return nil
}

// Install installs a package on the remote machine and runs it.
func (c *Client) Install(ctx context.Context, req *msgs.InstallReq) error {
	b, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("had problem marshaling install request: %w", err)
	}

	u := fmt.Sprintf("http://%s/api/v1.0.0/install", c.endpoint)

	httpReq, err := http.NewRequest(http.MethodPost, u, bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("had problem creating http request for install: %w", err)
	}
	httpReq = httpReq.WithContext(ctx)
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := c.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("had problem with install HTTP request: %w", err)
	}
	defer httpResp.Body.Close()
	if httpResp.StatusCode != http.StatusOK {
		b, err = io.ReadAll(httpResp.Body)
		if err != nil {
			return fmt.Errorf("had problem reading install HTTP response: %w", err)
		}
		resp := &msgs.InstallResp{}
		if err := json.Unmarshal(b, resp); err != nil {
			return fmt.Errorf("had problem unmarshaling install HTTP response: %w", err)
		}
		return fmt.Errorf("install failed: %s", resp.ErrMsg)
	}
	return nil
}
