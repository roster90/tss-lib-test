package main

import (
	"sync"

	"github.com/roster90/tss-lib-test/mpc/server"
)

var wg sync.WaitGroup

func main() {
	peers := map[string]string{
		"nodeA": "localhost:50051",
		"nodeB": "localhost:50052",
	}

	wg.Add(2)
	go server.StartNode("nodeA", ":50051", peers)
	go server.StartNode("nodeB", ":50052", peers)

	wg.Wait()
}
