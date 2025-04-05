package handler

import (
	"context"
	"log"
	"reflect"
	"strings"
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

// ProcessMessages processes outgoing messages from the party
func ProcessMessages(nodeID string, chOut chan tss.Message, partyID *tss.PartyID) {
	log.Printf("[DEBUG] Starting message handler for node %s", nodeID)

	for msg := range chOut {
		log.Printf("[DEBUG] Processing message from node %s", nodeID)
		log.Printf("[DEBUG] Message type: %T", msg)
		log.Printf("[DEBUG] Message content: Type: %s, From: %v, To: %v, Round: %d",
			reflect.TypeOf(msg).String(), msg.GetFrom(), msg.GetTo(), getMessageRound(msg))
		log.Printf("[DEBUG] Message from: %s", msg.GetFrom().Moniker)
		log.Printf("[DEBUG] Message to: %v", msg.GetTo())

		// Handle broadcast messages
		if len(msg.GetTo()) == 0 {
			log.Printf("[DEBUG] Processing broadcast message for Round %d", getMessageRound(msg))
			for peerID, peerAddr := range peers {
				if peerID != nodeID {
					log.Printf("[DEBUG] Sending broadcast to %s at %s for Round %d",
						peerID, peerAddr, getMessageRound(msg))
					if err := sendMessage(nodeID, peerID, peerAddr, msg); err != nil {
						log.Printf("[ERROR] Failed to send broadcast to %s for Round %d: %v",
							peerID, getMessageRound(msg), err)
					}
				}
			}
		} else {
			// Handle point-to-point messages
			log.Printf("[DEBUG] Processing point-to-point message for Round %d", getMessageRound(msg))
			for _, to := range msg.GetTo() {
				peerID := to.Moniker
				if peerID != nodeID {
					if peerAddr, ok := peers[peerID]; ok {
						log.Printf("[DEBUG] Sending message to %s at %s for Round %d",
							peerID, peerAddr, getMessageRound(msg))
						if err := sendMessage(nodeID, peerID, peerAddr, msg); err != nil {
							log.Printf("[ERROR] Failed to send message to %s for Round %d: %v",
								peerID, getMessageRound(msg), err)
						}
					} else {
						log.Printf("[ERROR] Could not find address for peer %s", peerID)
					}
				}
			}
		}
	}
}

// sendMessage sends a message to a specific node
func sendMessage(from, toID, toAddr string, msg tss.Message) error {
	log.Printf("[DEBUG] Sending message to %s at %s", toID, toAddr)
	log.Printf("[DEBUG] Message details - From: %s, To: %s, Type: %T, Round: %d",
		from, toID, msg, getMessageRound(msg))

	// Convert message to bytes
	msgBytes, _, err := msg.WireBytes()
	if err != nil {
		log.Printf("[ERROR] Failed to convert message to bytes: %v", err)
		return err
	}
	log.Printf("[DEBUG] Converted message to bytes, length: %d", len(msgBytes))

	// Create gRPC client with increased timeout and message size
	conn, err := grpc.Dial(toAddr,
		grpc.WithInsecure(),
		grpc.WithBlock(),
		grpc.WithTimeout(30*time.Second),
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

	log.Printf("[DEBUG] Sending Round %d message to %s", getMessageRound(msg), toID)
	_, err = client.ReceiveMessage(ctx, &proto.MessageRequest{
		From:    from,
		Message: msgBytes,
	})
	if err != nil {
		log.Printf("[ERROR] Failed to send Round %d message to %s: %v", getMessageRound(msg), toID, err)
		return err
	}

	log.Printf("[DEBUG] Successfully sent Round %d message to %s", getMessageRound(msg), toID)
	return nil
}

// getMessageRound extracts the round number from the message type
func getMessageRound(msg tss.Message) int {
	switch msg.(type) {
	case *tss.MessageImpl:
		// Extract round from message type string
		msgType := reflect.TypeOf(msg).String()
		switch {
		case strings.Contains(msgType, "KGRound1Message"):
			return 1
		case strings.Contains(msgType, "KGRound2Message"):
			return 2
		case strings.Contains(msgType, "KGRound3Message"):
			return 3
		default:
			return 0
		}
	default:
		return 0
	}
}

// ProcessErrors handles error messages from the party
func ProcessErrors(errCh chan error) {
	log.Printf("[DEBUG] Starting error handler")

	for err := range errCh {
		if err != nil {
			log.Printf("[ERROR] Party error: %v", err)
		}
	}
}
