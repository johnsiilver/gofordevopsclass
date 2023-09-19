package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"strings"
	"sync"
)

const (
	ping = 0
)

// program is a program name and path. path is setup by exec.Lookup().
type program struct {
	name string
	path string
}

var requiredList = []program{{name: "ping"}}

func programValidation() error {
	var err error
	for i, p := range requiredList {
		p.path, err = exec.LookPath(p.name)
		if err != nil {
			return fmt.Errorf("cannot find %s in our PATH", p)
		}
		requiredList[i] = p
	}
	return nil
}

// hostalive returns true if the host is alive. You require special privileges to send
// ICMP packets on most devices. This bypasses that requirement by using the ping command.
func hostAlive(ctx context.Context, host netip.Addr) bool {
	// -c 1: send 1 ping packet
	// -t 2: timeout after 2 seconds
	// Note: to work on all platforms (like windows), you might need to use
	// build tags around this function or use os detection via runtime.GOOS.
	cmd := exec.CommandContext(ctx, requiredList[ping].path, "-c", "1", "-t", "2", host.String())

	if err := cmd.Run(); err != nil {
		return false
	}
	return true
}

func main() {
	ctx := context.Background()
	programValidation()

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "-h", "--help":
			fmt.Println("usage: ping <host>")
			os.Exit(0)
		}
	}

	// Either get all the ip/dns names from stdin or use the command line arguments.
	wg := sync.WaitGroup{}
	ch := make(chan string, 1)
	if len(os.Args) < 2 { // stdin
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer close(ch)
			handleStdin(os.Stdin, ch)
		}()
	} else { // command line arguments
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer close(ch)
			for _, host := range os.Args[1:] {
				ch <- host
			}
		}()
	}

	// process the hosts
	for host := range ch {
		host := host

		addr, err := netip.ParseAddr(host)
		if err != nil {
			addrs, err := net.LookupHost(host)
			if err != nil {
				fmt.Printf("%q is not a valid IP or DNS name: %s\n", host, err)
				os.Exit(1)
			}
			addr = netip.MustParseAddr(addrs[0])
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			fmt.Printf("host(%s) is alive: %v\n", host, hostAlive(ctx, addr))
		}()
	}
	wg.Wait()
}

func handleStdin(r io.Reader, ch chan string) {
	reader := bufio.NewReader(r)

	for {
		// read line from stdin using newline as separator
		line, err := reader.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				log.Fatal(err)
			}
			break
		}

		line = strings.TrimSpace(line)
		// if line is empty, skip it
		if len(line) == 0 {
			continue
		}
		ch <- line
	}
}
