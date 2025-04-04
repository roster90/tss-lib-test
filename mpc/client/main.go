package main

import (
	"context"
	"log"
	"time"

	"github.com/roster90/tss-lib-test/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	// Connect to node A
	conn, err := grpc.Dial("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	client := proto.NewMPCServiceClient(conn)

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Call GenerateShares
	log.Printf("Calling GenerateShares on node A")
	resp, err := client.GenerateShares(ctx, &proto.GenerateSharesRequest{})
	if err != nil {
		log.Fatalf("GenerateShares failed: %v", err)
	}

	log.Printf("Public Key: %s", resp.PublicKey)
	log.Printf("Share: %s", resp.Shares)
	log.Printf("Share saved to shares/nodeA/share.json")
}
