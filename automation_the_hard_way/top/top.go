// Get's the output of the top command and prints it to stdout.
package main

import (
	"fmt"
	"log"
	"os/exec"
)

func main() {
	cmd := exec.Command("top", "-l", "1")

	b, err := cmd.CombinedOutput()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(string(b))
}
