package server

import (
	"context"
	"fmt"
	"log"
	"reflect"
	"strings"
	"time"

	tssLib "github.com/bnb-chain/tss-lib/v2/tss"
	"github.com/roster90/tss-lib-test/mpc/handler"
	"github.com/roster90/tss-lib-test/mpc/tss"
	"github.com/roster90/tss-lib-test/proto"
)

// GenerateShares implements the gRPC service method
func (s *MPCServer) GenerateShares(ctx context.Context, req *proto.GenerateSharesRequest) (*proto.GenerateSharesResponse, error) {
	log.Printf("[DEBUG] GenerateShares called for party %s (ID: %s, Index: %d)", s.partyID.Moniker, s.partyID.Id, s.partyID.Index)

	// Initialize party and channels
	party, chOut, endCh, errCh := tss.InitLocalParty(s.params)

	// Start error handler
	go handler.ProcessErrors(errCh)
	log.Printf("[DEBUG] Initialized new party for keygen with params: threshold=%d, partyCount=%d", s.params.Threshold(), len(s.partyIDs))

	// Start message handler
	go handler.ProcessMessages(s.nodeID, chOut, s.partyID, "default")

	// Start keygen process
	log.Printf("[DEBUG] Starting keygen process for party %s", s.partyID)
	if err := party.Start(); err != nil {
		log.Printf("[ERROR] Failed to start keygen: %v", err)
		return nil, fmt.Errorf("failed to start keygen: %v", err)
	}
	log.Printf("[DEBUG] Started keygen process successfully")

	// Wait for keygen to complete
	log.Printf("[DEBUG] Waiting for keygen to complete...")
	select {
	case <-ctx.Done():
		log.Printf("[DEBUG] Context cancelled while waiting for keygen")
		return nil, ctx.Err()
	case err := <-errCh:
		log.Printf("[ERROR] Keygen failed: %v", err)
		return nil, fmt.Errorf("keygen failed: %v", err)
	case data := <-endCh:
		if data == nil {
			log.Printf("[ERROR] No data received from endCh")
			return nil, fmt.Errorf("no data received")
		}

		log.Printf("[DEBUG] Received keygen data: Xi=%s, PubKey=(%s,%s)",
			data.Xi.String(),
			data.ECDSAPub.X().String(),
			data.ECDSAPub.Y().String())

		// Convert to string format
		pubKeyStr := fmt.Sprintf("%s,%s", data.ECDSAPub.X().String(), data.ECDSAPub.Y().String())
		sharesStr := data.Xi.String()

		// Save share to JSON file
		if err := tss.SaveShare(s.nodeID, sharesStr, pubKeyStr); err != nil {
			log.Printf("[ERROR] Failed to save share: %v", err)
			return nil, fmt.Errorf("failed to save share: %v", err)
		}

		log.Printf("[INFO] Successfully generated and saved share for node %s", s.nodeID)
		return &proto.GenerateSharesResponse{
			PublicKey: pubKeyStr,
			Shares:    sharesStr,
		}, nil
	case <-time.After(60 * time.Second):
		log.Printf("[ERROR] Timeout waiting for keygen completion after 60 seconds")
		return nil, fmt.Errorf("keygen not completed yet")
	}
}

// ReceiveMessage implements the gRPC service method for receiving messages from other nodes
func (s *MPCServer) ReceiveMessage(ctx context.Context, req *proto.MessageRequest) (*proto.MessageResponse, error) {
	log.Printf("[DEBUG] Received message from %s", req.From)

	// Find the sender's PartyID
	senderPartyID := findPartyID(req.From, s.partyIDs)
	if senderPartyID == nil {
		log.Printf("[ERROR] Could not find PartyID for sender: %s", req.From)
		return nil, fmt.Errorf("invalid sender: %s", req.From)
	}

	log.Printf("[DEBUG] Processing message from %s (Index: %d, ID: %s)",
		req.From, senderPartyID.Index, senderPartyID.Id)
	log.Printf("[DEBUG] Message content length: %d bytes", len(req.Message))

	// Parse and process the message
	msg, err := tssLib.ParseWireMessage(req.Message, senderPartyID, true)
	if err != nil {
		log.Printf("[ERROR] Failed to parse message: %v", err)
		return nil, fmt.Errorf("failed to parse message: %v", err)
	}

	// Initialize party if not already initialized
	party, chOut, _, errCh := tss.InitLocalParty(s.params)

	// Start error handler
	go handler.ProcessErrors(errCh)
	log.Printf("[DEBUG] Initialized new party for keygen with params: threshold=%d, partyCount=%d", s.params.Threshold(), len(s.partyIDs))

	// Start message handler
	go handler.ProcessMessages(s.nodeID, chOut, s.partyID, "default")

	// Start keygen process
	log.Printf("[DEBUG] Starting keygen process for node %s", s.nodeID)
	if err := party.Start(); err != nil {
		log.Printf("[ERROR] Failed to start keygen for node %s: %v", s.nodeID, err)
		return nil, fmt.Errorf("failed to start keygen: %v", err)
	}
	log.Printf("[DEBUG] Started keygen process for node %s", s.nodeID)

	// Process the message
	ok, err := party.Update(msg)
	if !ok {
		log.Printf("[ERROR] Failed to update party: %v", err)
		return nil, fmt.Errorf("failed to update party: %v", err)
	}

	log.Printf("[DEBUG] Successfully processed message from %s", req.From)
	return &proto.MessageResponse{}, nil
}

// getCurrentRound returns the current round number of the party
func getCurrentRound(party tssLib.Party) int {
	if party == nil {
		log.Printf("[DEBUG] Party is nil")
		return 0
	}

	// Get party type
	partyType := reflect.TypeOf(party).String()
	log.Printf("[DEBUG] Party type: %s", partyType)

	// Check if it's a keygen party
	if strings.Contains(partyType, "keygen.LocalParty") {
		// If party is initialized and has a PartyID
		if party.PartyID() != nil {
			// Try to get round from message type
			if msg, ok := party.(interface{ GetMessage() tssLib.Message }); ok {
				if msg.GetMessage() != nil {
					msgType := msg.GetMessage().Type()
					log.Printf("[DEBUG] Current message type: %s", msgType)

					switch {
					case strings.Contains(msgType, "KGRound1Message"):
						return 1
					case strings.Contains(msgType, "KGRound2Message"):
						return 2
					case strings.Contains(msgType, "KGRound3Message"):
						return 3
					}
				}
			}

			// If we can't determine round from message, check if we've processed any messages
			if msgs := getProcessedMessages(party); len(msgs) > 0 {
				lastMsg := msgs[len(msgs)-1]
				msgType := lastMsg.Type()
				log.Printf("[DEBUG] Last processed message type: %s", msgType)

				switch {
				case strings.Contains(msgType, "KGRound1Message"):
					return 2 // If we've processed round 1 messages, we're in round 2
				case strings.Contains(msgType, "KGRound2Message"):
					return 3 // If we've processed round 2 messages, we're in round 3
				}
			}

			log.Printf("[DEBUG] Party is initialized but no messages processed yet")
			return 1
		}
		log.Printf("[DEBUG] Party is not fully initialized")
		return 0
	}

	// For other party types, try to determine round from type name
	switch {
	case strings.Contains(partyType, "round1"):
		return 1
	case strings.Contains(partyType, "round2"):
		return 2
	case strings.Contains(partyType, "round3"):
		return 3
	default:
		log.Printf("[DEBUG] Unknown party type: %s", partyType)
		return 0
	}
}

// Helper function to get processed messages
func getProcessedMessages(party tssLib.Party) []tssLib.Message {
	var messages []tssLib.Message

	// Try to get messages from party's state
	if state, ok := party.(interface{ GetState() interface{} }); ok {
		if stateValue := state.GetState(); stateValue != nil {
			if msgs, ok := stateValue.([]tssLib.Message); ok {
				return msgs
			}
		}
	}

	return messages
}

// Helper function to generate a unique session ID
func generateSessionID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
