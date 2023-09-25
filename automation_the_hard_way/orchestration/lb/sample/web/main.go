package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/google/uuid"
)

var (
	node = flag.String("node", uuid.New().String(), "The node name")
	port = flag.Int("port", 8082, "The port to run on")
)

func main() {
	flag.Parse()

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Hello web from node "+*node)
	})
	http.HandleFunc("/installedAt", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, os.Args[0])
	})
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "ok")
	})
	log.Println("running on port: ", *port)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *port), nil))
}
