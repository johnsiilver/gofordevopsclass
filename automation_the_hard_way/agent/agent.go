package main

import (
	"flag"
	"log"

	"github.com/gin-gonic/gin"
	"github.com/johnsiilver/gofordevopsclass/automation_the_hard_way/agent/service"
)

var (
	addr     = flag.String("addr", "localhost:8080", "address to listen on")
	qotdAddr = flag.String("qotdAddr", ":17", "the addrss to run the qotd service on")
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	flag.Parse()

	agent, err := service.New(gin.Default(), *addr)
	if err != nil {
		log.Fatalf("unable to create agent: %s", err)
	}
	agent.QOTDAddr = *qotdAddr
	if err := agent.Start(); err != nil {
		log.Fatalf("unable to start agent: %s", err)
	}
}
