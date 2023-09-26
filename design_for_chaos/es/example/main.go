package main

import (
	"log"
	"time"

	"github.com/johnsiilver/gofordevopsclass/design_for_chaos/es"
)

func main() {
	if es.Data.Status("SatelliteDiskErase") != es.Go {
		log.Fatalf("emergency stop is in effect before doing anything")
	}

	go func() {
		for {
			if es.Data.Status("SatelliteDiskErase") != es.Go {
				log.Fatalf("emergency stop is in effect")
			}
			time.Sleep(10 * time.Second)
		}
	}()

	for i := 0; i < 100; i++ {
		log.Println("happily disk erasing")
		time.Sleep(1 * time.Second)
	}
}

/*
func main() {
	statusCh, esCancel := es.Data.Subscribe("SatelliteDiskErase")
	defer esCancel()

	if <-statusCh != es.Go {
		log.Fatalf("emergency stop is in effect before doing anything")
	}

	done := make(chan struct{})
	defer close(done)

	go func() {
		for {
			select {
			case <-done:
				return
			case status := <-statusCh:
				if status != es.Go {
					log.Fatalf("emergency stop is in effect")
				}
			}
		}
	}()

	for i := 0; i < 100; i++ {
		log.Println("happily disk erasing")
		time.Sleep(1 * time.Second)
	}
}
*/
