package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/netip"
	"os"
	"strings"
)

const usage = `
Usage: qotd <addr> [author]

addr:   The address of the QOTD server. If no port is specified, port 17
		will be used. This must be a valid IP address, not a hostname.
author: The author of the quote to request. If the author is "none", a random
		quote will be returned. If no author is specified, the client will
		read from stdin and write to the connection. You have 5 milliseconds
		to type a quote before the connection is closed, so be quick!
`

func main() {
	if len(os.Args) == 1 {
		log.Fatal(usage)
	}

	for _, arg := range os.Args[1:] {
		if arg == "-h" || arg == "-help" {
			fmt.Printf(usage)
			os.Exit(0)
		}
	}

	// Parse the address from the command line.
	// Note that with os.Args, the first argument is the program name as
	// is standard with Unix. Note that this is not the case with Cobra.
	addr, err := getAddr(os.Args[1])
	if err != nil {
		log.Fatalf("invalid remote address(%s): %s", os.Args[1], err)
	}

	conn, err := net.Dial("tcp", addr.String())
	if err != nil {
		log.Fatal(err)
	}

	switch len(os.Args) {
	case 2:
		handleStdin(conn)
	default:
		author := strings.Join(os.Args[2:], " ")
		handleAuthor(conn, author)
	}
}

func getAddr(s string) (netip.AddrPort, error) {
	if strings.Contains(s, ":") {
		return netip.ParseAddrPort(s)
	}
	return netip.ParseAddrPort(s + ":17")
}

func handleStdin(conn net.Conn) {
	defer conn.Close()

	// Read from stdin and write to the connection.
	_, err := io.Copy(conn, os.Stdin)
	if err != nil {
		log.Fatal(err)
	}

	// Read from the connection and write to stdout.
	_, err = io.Copy(os.Stdout, conn)
	if err != nil {
		log.Fatal(err)
	}
	os.Stdout.WriteString("\n")
}

func handleAuthor(conn net.Conn, author string) {
	defer conn.Close()

	if author != "none" {
		// Write the author to the connection.
		_, err := io.WriteString(conn, author+"\n")
		if err != nil {
			log.Fatal(err)
		}
	}

	// Read from the connection and write to stdout.
	_, err := io.Copy(os.Stdout, conn)
	if err != nil {
		log.Fatal(err)
	}
	os.Stdout.WriteString("\n")
}
