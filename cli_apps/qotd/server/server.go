package main

import (
	"bytes"
	"flag"
	"log"
	"math/rand"
	"net"
	"time"
	"unsafe"
)

var (
	port = flag.String("port", ":17", "port to listen on")
)

var authorToQuotes = map[string][]string{
	"mark twain": {
		"History doesn't repeat itself, but it does rhyme",
		"Lies, damned lies, and statistics",
		"Golf is a good walk spoiled",
	},
	"benjamin franklin": {
		"Tell me and I forget. Teach me and I remember. Involve me and I learn",
		"I didn't fail the test. I just found 100 ways to do it wrong",
	},
	"eleanor roosevelt": {
		"The future belongs to those who believe in the beauty of their dreams",
	},
	"helen keller": {
		"Keep your face to the sunshine and you cannot see a shadow",
		"The chief handicap of the blind is not blindness, but the attitude of seeing people towards them",
		"The true test of a character is to face hard conditions with the determination to make them better",
	},
	"maya angelou": {
		"I've learned that people will forget what you said, people will forget what you did, but people will never forget how you made them feel",
		"If you don't like something, change it. If you can't change it, change your attitude",
		"Nothing will work unless you do",
	},
}

func main() {
	flag.Parse()

	log.Println("Starting QOTD server on port", *port)

	addr, err := net.ResolveTCPAddr("tcp4", *port)
	if err != nil {
		log.Fatal(err)
	}

	listen, err := net.ListenTCP("tcp", addr)
	if err != nil {
		log.Fatal(err)
	}
	defer listen.Close()

	maxConn := make(chan struct{}, 100)
	for {
		conn, err := listen.Accept()
		if err != nil {
			continue
		}

		go func() {
			defer conn.Close()

			// Rate limit service to 100 concurrent connections.
			select {
			case maxConn <- struct{}{}:
				defer func() { <-maxConn }()
			default:
				log.Println("Max connections reached")
				return
			}

			log.Println("Connection from", conn.RemoteAddr())

			conn.SetReadDeadline(time.Now().Add(5 * time.Millisecond))
			buf := make([]byte, 1024)
			n, err := conn.Read(buf)
			if err == nil {
				if bytes.HasSuffix(buf[:n], []byte("\n")) {
					var b []byte
					// Get the author name from the client and convert it to
					// a string without a copy.
					buf = bytes.TrimSpace(buf[:n])
					author := unsafe.String(&buf[0], len(buf))
					log.Println("requested author:", author)
					if quotes, ok := authorToQuotes[author]; ok {
						// Pick a random quote from the author, convert it to
						// a string without a copy, and write it to the client.
						q := quotes[rand.Intn(len(quotes))]
						b = unsafe.Slice(unsafe.StringData(q), len(q))
					} else {
						log.Printf("Author was %q", author)
						b = []byte("Author not found\n")
					}
					conn.SetWriteDeadline(time.Now().Add(1 * time.Second))
					conn.Write(b)
					return
				}
			}
			for _, quotes := range authorToQuotes {
				q := quotes[rand.Intn(len(quotes))]
				b := unsafe.Slice(unsafe.StringData(q), len(q))
				conn.SetWriteDeadline(time.Now().Add(1 * time.Second))
				conn.Write(b)
				return
			}
		}()
	}
}
