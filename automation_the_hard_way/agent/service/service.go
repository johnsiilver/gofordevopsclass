package service

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"

	"github.com/gin-contrib/expvar"
	"github.com/gin-gonic/gin"
	"github.com/johnsiilver/gofordevopsclass/automation_the_hard_way/agent/msgs"
)

const (
	// pkgDir is the directory in the Agent user's home where we are installing and
	// running packages. A more secure version would be to have the agent do this
	// in individual user directories that match some user on all machines. However
	// this is for illustration purposes only.
	pkgDir = "sa/packages/"

	maxInstallSize = 1 * 1024 * 1024 * 1024 // 1GB
)

// Agent provides a simple System Agent using REST to install and run programs.
type Agent struct {
	// homePath is the path to the user's home directory.
	homePath string
	// addr is the address to listen on.
	addr   string
	router *gin.Engine

	// cpuData is the atomic pointer to the CPU data.
	cpuData atomic.Pointer[msgs.CPUPerfs]
	// memData is the atomic pointer to the memory data.
	memData atomic.Pointer[msgs.MemPerf]
}

// New creates a new Agent. If addr is empty, it will default to localhost:8080.
func New(router *gin.Engine, addr string) (*Agent, error) {
	u, err := user.Current()
	if err != nil {
		return nil, err
	}
	if addr == "" {
		addr = "localhost:8080"
	}

	homePath := ""
	switch runtime.GOOS {
	case "linux":
		homePath = filepath.Join("/home", u.Username)
	case "darwin":
		homePath = filepath.Join("/Users", u.Username)
	default:
		return nil, fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}

	agent := &Agent{
		homePath: homePath,
		router:   router,
		addr:     addr,
	}

	if err := agent.perfLoop(); err != nil {
		return nil, fmt.Errorf("problem starting perf loop: %w", err)
	}

	router.GET("/debug/vars", expvar.Handler())
	router.POST("/api/v1.0.0/install", agent.Install)
	return agent, nil
}

// Install installs a package on the machine and starts it.
func (a *Agent) Install(c *gin.Context) {
	req, err := a.getInstallReq(*c.Request)
	if err != nil {
		sendInstallError(c, http.StatusBadRequest, err)
		return
	}

	from, err := a.unpack(req)
	if err != nil {
		sendInstallError(c, http.StatusBadRequest, err)
		return
	}
	if err := a.migrate(req, from); err != nil {
		sendInstallError(c, http.StatusBadRequest, err)
		return
	}
	if err := a.startProgram(c.Request.Context(), req); err != nil {
		sendInstallError(c, http.StatusBadRequest, err)
		return
	}

	c.IndentedJSON(http.StatusOK, msgs.InstallResp{})
}

// getInstallReq gets the msgs.InstallReq from the request body. It will return
// an error if the body is larger than maxInstallSize (1 GiB). Also runs the
// validation on the message.
func (a *Agent) getInstallReq(r http.Request) (*msgs.InstallReq, error) {
	lr := io.LimitedReader{
		R: r.Body,
		N: maxInstallSize,
	}
	defer r.Body.Close()

	b, err := io.ReadAll(&lr)
	if err != nil {
		return nil, fmt.Errorf("unable to read message body: %s", err)
	}

	req := &msgs.InstallReq{}
	if err := json.Unmarshal(b, req); err != nil {
		return nil, fmt.Errorf("unable to unmarshal message body: %s", err)
	}

	return req, req.Validate()
}

// unpack unpacks the zip file into a temp directory and returns the directory location.
func (a *Agent) unpack(req *msgs.InstallReq) (string, error) {
	dir, err := os.MkdirTemp("", fmt.Sprintf("sa_install_%s_*", req.Name))
	if err != nil {
		return "", err
	}
	r, err := zip.NewReader(bytes.NewReader(req.Package), int64(len(req.Package)))
	if err != nil {
		return "", err
	}

	// Iterate through the files in the archive, writing the files into our
	// temp directory.
	for _, f := range r.File {
		if err := a.writeFile(f, dir); err != nil {
			return "", err
		}
	}
	return dir, nil
}

// writeFile writes a zip file under the root directory dir.
func (a *Agent) writeFile(z *zip.File, dir string) error {
	if z.FileInfo().IsDir() {
		err := os.Mkdir(
			filepath.Join(dir, filepath.FromSlash(z.Name)),
			z.Mode(),
		)
		return err
	}

	rc, err := z.Open()
	if err != nil {
		return fmt.Errorf("could not open file %q: %w", z.Name, err)
	}
	defer rc.Close()

	nf, err := os.OpenFile(
		filepath.Join(dir, filepath.FromSlash(z.Name)),
		os.O_CREATE|os.O_WRONLY,
		z.Mode(),
	)
	if err != nil {
		return fmt.Errorf("could not open file in temp diretory: %w", err)
	}
	defer nf.Close()

	_, err = io.Copy(nf, rc)
	if err != nil {
		return fmt.Errorf("file copy error: %w", err)
	}
	return nil
}

// migrate shuts down any existing job that is running and migrates our files
// from the temp location to the final location.
func (a *Agent) migrate(req *msgs.InstallReq, from string) error {
	to := filepath.Join(a.homePath, pkgDir, req.Name)
	// We can only have one program running at a time.
	// Note: I did not implement something to stop any existing programs that are running.
	// This is trivial to implement.
	if _, err := os.Stat(to); err == nil {
		os.RemoveAll(to)
	}
	log.Println("from: ", from)
	log.Println("to: ", to)
	if err := os.Rename(from, to); err != nil {
		return err
	}
	return nil
}

// startProgram starts our program manually.
// Note: You generally want to use a process manager like systemd, upstart, etc to manage
//
//	your programs. This is for illustration purposes only. You would also want something
//	to restart the program if it dies by monitoring the process.
func (a *Agent) startProgram(ctx context.Context, req *msgs.InstallReq) error {
	p := filepath.Join(a.homePath, pkgDir)
	cmdStr := fmt.Sprintf(
		"%s %s &",
		filepath.Join(p, req.Name, req.Binary),
		strings.Join(req.Args, " "),
	)
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", cmdStr)

	cmd.Stderr = nil
	cmd.Stdout = nil
	if err := cmd.Start(); err != nil {
		return err
	}
	log.Println("Started program: ", cmd.Process.Pid)
	log.Println("waiting: ", cmd.Wait())
	return nil
}

func sendInstallError(c *gin.Context, status int, err error) {
	c.IndentedJSON(
		status,
		msgs.InstallResp{
			ErrMsg: err.Error(),
		},
	)
}
