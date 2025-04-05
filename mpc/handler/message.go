package handler

import (
	"context"
	"log"

	"github.com/bnb-chain/tss-lib/v2/tss"
	"github.com/roster90/tss-lib-test/proto"
	"google.golang.org/grpc"
)

// ProcessMessages handles incoming messages from the TSS party
func ProcessMessages(nodeID string, chOut chan tss.Message, partyID *tss.PartyID, sessionID string) {
	log.Printf("[DEBUG] Starting message handler for node %s", nodeID)

	for msg := range chOut {
		log.Printf("[DEBUG] Processing message of type %s", msg.Type())

		// Get message bytes
		msgBytes, _, err := msg.WireBytes()
		if err != nil {
			log.Printf("[ERROR] Failed to get wire bytes: %v", err)
			continue
		}

		// Send message to the other node
		peerAddr := "localhost:50051"
		if nodeID == "nodeA" {
			peerAddr = "localhost:50052"
		}

		// Create gRPC client
		conn, err := grpc.Dial(peerAddr, grpc.WithInsecure())
		if err != nil {
			log.Printf("[ERROR] Failed to connect to peer %s: %v", peerAddr, err)
			continue
		}
		defer conn.Close()

		client := proto.NewMPCServiceClient(conn)

		// Send message
		_, err = client.ReceiveMessage(context.Background(), &proto.MessageRequest{
			From:      nodeID,
			Message:   msgBytes,
			SessionId: sessionID,
		})
		if err != nil {
			log.Printf("[ERROR] Failed to send message to %s: %v", peerAddr, err)
			continue
		}

		log.Printf("[DEBUG] Successfully sent message to %s", peerAddr)
	}
}
