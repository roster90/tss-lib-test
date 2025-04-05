package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/roster90/tss-lib-test/mpc/server"
)

func main() {
	nodeName := flag.String("node", "", "Node name to run (A or B)")
	port := flag.String("port", "", "Port to listen on")
	flag.Parse()

	if *nodeName == "" || *port == "" {
		fmt.Println("Usage: ./tss-lib-test -node <A|B> -port <port>")
		os.Exit(1)
	}

	var nodeID string
	switch *nodeName {
	case "A":
		nodeID = "nodeA"
	case "B":
		nodeID = "nodeB"
	default:
		fmt.Println("Invalid node name. Please use either A or B")
		os.Exit(1)
	}

	server.StartNode(nodeID, ":"+*port)
}
