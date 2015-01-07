package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) > 1 {
		if os.Args[1] == "client" {
			Client()
			return
		} else if os.Args[1] == "server" {
			Server()
			return
		}
	}

	fmt.Fprintf(os.Stderr, "usage: %s client|server\n", os.Args[0])
	os.Exit(2)
}
