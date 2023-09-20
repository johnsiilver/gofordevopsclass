//go:build ds

package client

import (
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/johnsiilver/serveonssh"
	"golang.org/x/crypto/ssh"
)

// New creates a new Client that connects to a remote endpoint via SSH and then
// uses that connection to dial into a domain socket the agent is using. The
// gRPC client actually uses a domain socket on this side which is then forwarded
// over SSH. endpoint is the host:port of the remote endpoint.
func New(endpoint string, auth []ssh.AuthMethod) (*Client, error) {
	config := &ssh.ClientConfig{
		User:            os.Getenv("USER"),
		Auth:            auth,
		Timeout:         5 * time.Second,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	remoteSocket := filepath.Join("/home", config.User, "/sa/socket/sa.sock")

	p, err := serveonssh.New(endpoint, remoteSocket, config)
	if err != nil {
		return nil, err
	}

	client := &http.Client{}

	return &Client{
		endpoint: endpoint,
		client:   client,
		p:        p,
	}, nil
}
