package handler

import (
	"context"
	"fmt"
	"log"
	"math/big"
	"time"

	tss "github.com/bnb-chain/tss-lib/v2/tss"
	pd "github.com/roster90/tss-lib-test/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// HandleMessages sends/receives messages between nodes via gRPC
func HandleMessages(nodeID string, peers map[string]string, chOut chan tss.Message, fromPartyID *tss.PartyID) {
	log.Printf("Starting message handler for %s", nodeID)

	// Initialize mappings between party IDs and node identifiers
	idToMoniker := map[string]string{"1": "nodeA", "2": "nodeB"}
	monikerToId := map[string]string{"nodeA": "1", "nodeB": "2"}

	// Handle each outgoing message in the channel
	for msg := range chOut {
		recipients := msg.GetTo()
		isBroadcast := len(recipients) == 0

		var targets []*tss.PartyID
		if isBroadcast {
			// If it's a broadcast, send to all peers except the sender
			log.Printf("Broadcasting from %s to all peers", nodeID)
			targets = getBroadcastRecipients(peers, nodeID, monikerToId)
		} else {
			// Otherwise, send to specific recipients
			targets = recipients
		}

		// Prepare the message and send it
		err := sendMessageToPeers(nodeID, peers, targets, msg, idToMoniker, monikerToId)
		if err != nil {
			log.Printf("Failed to send message from %s: %v", nodeID, err)
		}
	}
}

// getBroadcastRecipients returns a list of PartyIDs for all peers except the sender
func getBroadcastRecipients(peers map[string]string, nodeID string, monikerToId map[string]string) []*tss.PartyID {
	var targets []*tss.PartyID
	for peerMoniker := range peers {
		if peerMoniker != nodeID {
			id := monikerToId[peerMoniker]
			targets = append(targets, tss.NewPartyID(id, peerMoniker, big.NewInt(0)))
		}
	}
	return targets
}

// sendMessageToPeers sends a message to a list of recipients (peers)
func sendMessageToPeers(nodeID string, peers map[string]string, targets []*tss.PartyID, msg tss.Message, idToMoniker map[string]string, monikerToId map[string]string) error {
	// Serialize the message
	msgBytes, _, err := msg.WireBytes()
	if err != nil {
		return fmt.Errorf("failed to serialize message from %s: %v", nodeID, err)
	}
	log.Printf("Message prepared from %s: Type=%s", nodeID, msg.Type())

	// For each recipient, send the message
	for _, peerID := range targets {
		moniker := idToMoniker[peerID.Id]
		if moniker == "" {
			log.Printf("Unknown peer ID: %s", peerID.Id)
			continue
		}
		peerAddr := peers[moniker]

		// Retry message sending for each peer
		err := retrySendingMessage(context.Background(), nodeID, moniker, peerAddr, msgBytes, msg.Type())
		if err != nil {
			log.Printf("Failed to send message from %s to %s: %v", nodeID, moniker, err)
		}
	}
	return nil
}

// retrySendingMessage handles retries for sending a message to a peer
func retrySendingMessage(ctx context.Context, from, to, peerAddr string, message []byte, msgType string) error {
	for attempt := 1; attempt <= 3; attempt++ {
		if err := sendMessageToPeer(ctx, from, to, peerAddr, message); err == nil {
			log.Printf("Sent message from %s to %s (Type: %s)", from, to, msgType)
			return nil
		} else {
			log.Printf("Attempt %d: Failed to send message to %s: %v", attempt, to, err)
			time.Sleep(2 * time.Second)
		}
	}
	return fmt.Errorf("failed to send message after 3 attempts")
}

// sendMessageToPeer sends a message to a peer
func sendMessageToPeer(ctx context.Context, from, to, peerAddr string, message []byte) error {
	conn, err := grpc.Dial(peerAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("failed to connect to %s: %v", peerAddr, err)
	}
	defer conn.Close()

	client := pd.NewMPCServiceClient(conn)
	_, err = client.ReceiveMessage(ctx, &pd.MessageRequest{
		From:    from,
		To:      to,
		Message: message,
	})
	return err
}
