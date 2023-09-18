package main

import (
	"flag"
	"io"
	"log"
	"net"
	"net/netip"
	"os"
	"strings"
)

var (
	address = flag.String("addr", "127.0.0.1:17", "address of the QOTD server, must be ip:port")
	author  = flag.String("author", "none", "author of the quote to request")
)

const usage = `
Usage: qotd [flags]
`

func main() {
	// This provides a custom usage message when a user runs the program
	// with the -h or -help flags. It will also display if the user
	// provides an invalid flag.
	flag.Usage = func() {
		io.WriteString(os.Stderr, usage)
		flag.PrintDefaults()
	}
	flag.Parse()

	// Parse the address given with our --address flag.
	addr, err := getAddr(*address)
	if err != nil {
		log.Fatalf("invalid remote address(%s): %s", *address, err)
	}

	conn, err := net.Dial("tcp", addr.String())
	if err != nil {
		log.Fatal(err)
	}

	switch *author {
	case "stdin":
		handleStdin(conn)
	default:
		handleAuthor(conn, *author)
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
