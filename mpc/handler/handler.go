package handler

import (
	"context"
	"log"
	"time"

	"github.com/bnb-chain/tss-lib/v2/tss"
	"github.com/roster90/tss-lib-test/proto"
	"google.golang.org/grpc"
)

// Define peer addresses with proper connection details
var peers = map[string]string{
	"nodeA": "localhost:50051",
	"nodeB": "localhost:50052",
}

// // ProcessMessages processes outgoing messages from the party
// func ProcessMessages(nodeID string, chOut chan tss.Message, partyID *tss.PartyID, sessionID string) {
// 	log.Printf("[DEBUG] Starting message handler for node %s", nodeID)

// 	for msg := range chOut {

// 		// Handle broadcast messages
// 		if len(msg.GetTo()) == 0 {
// 			for peerID, peerAddr := range peers {
// 				if peerID != nodeID {

// 					if err := sendMessage(nodeID, peerID, peerAddr, msg, sessionID); err != nil {
// 						log.Printf("[ERROR] Failed to send broadcast to %s: %v", peerID, err)

// 					}
// 				}
// 			}
// 		} else {
// 			// Handle point-to-point messages
// 			for _, to := range msg.GetTo() {
// 				peerID := to.Moniker
// 				if peerID != nodeID {
// 					if peerAddr, ok := peers[peerID]; ok {

// 						if err := sendMessage(nodeID, peerID, peerAddr, msg, sessionID); err != nil {
// 							log.Printf("[ERROR] Failed to send message to %s: %v", peerID, err)

// 						}
// 					} else {
// 						log.Printf("[ERROR] Could not find address for peer %s", peerID)
// 					}
// 				}
// 			}
// 		}
// 	}
// }

// sendMessage sends a message to a specific node
func sendMessage(from, toID, toAddr string, msg tss.Message, sessionID string) error {

	// Convert message to bytes
	msgBytes, _, err := msg.WireBytes()
	if err != nil {
		log.Printf("[ERROR] Failed to convert message to bytes: %v", err)
		return err
	}

	// Create gRPC client with increased timeout and message size
	conn, err := grpc.Dial(toAddr,
		grpc.WithInsecure(),
		grpc.WithBlock(),
		grpc.WithTimeout(60*time.Second),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(1024*1024*1024))) // 1GB max message size
	if err != nil {
		log.Printf("[ERROR] Failed to connect to %s: %v", toAddr, err)
		return err
	}
	defer conn.Close()

	// Create client and send message with increased timeout
	client := proto.NewMPCServiceClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err = client.ReceiveMessage(ctx, &proto.MessageRequest{
		From:      from,
		To:        toID,
		Message:   msgBytes,
		SessionId: sessionID,
	})
	if err != nil {
		return err
	}

	return nil
}

// ProcessErrors handles error messages from the party
func ProcessErrors(errCh chan error) {

	for err := range errCh {
		if err != nil {
			log.Printf("[ERROR] Party error: %v", err)
		}
	}
}
