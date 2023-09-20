//go:build !ds

package service

// Start starts the agent.
func (a *Agent) Start() error {
	return a.router.Run(a.addr)
}
