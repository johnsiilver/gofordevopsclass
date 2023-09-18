package main

import (
	"flag"
	"io"
	"log"
	"net"
	"net/netip"
	"os"
	"strings"

	"github.com/fatih/color"
)

var (
	address = flag.String("addr", "127.0.0.1:17", "address of the QOTD server, must be ip:port")
	random  = flag.Bool("random", false, "get a quote from a random author")
)

const usage = `
Usage: qotd [author] [flags]

author: The author of the quote to request. If no author is specified, the client will
		read from stdin and write to the connection. You have 5 milliseconds
		to type a quote before the connection is closed, so be quick!

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

	author := strings.TrimSpace(strings.Join(flag.Args(), " "))

	if *random {
		if len(author) > 0 {
			log.Fatalf("cannot specify author(%s) with -random flag", author)
		}
	}

	// Parse the address from the command line.
	addr, err := getAddr(*address)
	if err != nil {
		log.Fatalf("invalid remote address(%s): %s", os.Args[1], err)
	}

	conn, err := net.Dial("tcp", addr.String())
	if err != nil {
		log.Fatal(err)
	}

	switch {
	case *random || len(author) > 1:
		if *random {
			author = "none"
		}
		handleAuthor(conn, author)
	default:
		handleStdin(conn)
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
	b := make([]byte, 1024)
	n, err := io.ReadFull(conn, b)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		log.Fatal(err)
	}
	color.Blue(string(b[:n]))
}
