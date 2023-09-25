package main

import (
	"flag"
	"log"
	"net"

	"github.com/johnsiilver/gofordevopsclass/automation_the_hard_way/orchestration/lb/server/grpc"
	"github.com/johnsiilver/gofordevopsclass/automation_the_hard_way/orchestration/lb/server/http"
)

var (
	httpAddr = flag.String("httpAddr", "localhost:9090", "The addr:port to listen on for HTTP requests for load-balancing")
	grpcAddr = flag.String("grpcAddr", "localhost:9091", "The addr:port to listen on for gRPC control requests")
)

func main() {
	flag.Parse()

	ln, err := net.Listen("tcp", *httpAddr)
	if err != nil {
		panic(err)
	}

	lb, err := http.New()
	if err != nil {
		panic(err)
	}

	log.Printf("load balancer started(%s)...", *httpAddr)
	go func() {
		if err := lb.Serve(ln); err != nil {
			panic(err)
		}
	}()

	serv, err := grpc.New(*grpcAddr, lb)
	if err != nil {
		panic(err)
	}

	log.Printf("grpc server started(%s)...", *grpcAddr)
	if err := serv.Start(); err != nil {
		panic(err)
	}
}
