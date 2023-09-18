package main

import (
	"io"
	"log"
	"net"
	"net/netip"
	"os"
	"strings"
)

/*
Put your flags and usage here
*/

func main() {
	/*
		Put your args/flags validation code here.
	*/

	// Parse the address from the command line.
	addr, err := getAddr(*address)
	if err != nil {
		log.Fatalf("invalid remote address(%s): %s", os.Args[1], err)
	}

	conn, err := net.Dial("tcp", addr.String())
	if err != nil {
		log.Fatal(err)
	}

	/*
		Put code here that handles calling handleAuthor and handleStdin
	*/
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
