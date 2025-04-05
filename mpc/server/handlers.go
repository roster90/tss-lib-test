package server

import (
	"context"
	"fmt"
	"log"
	"reflect"
	"strings"
	"time"

	"github.com/bnb-chain/tss-lib/v2/ecdsa/keygen"
	tssLib "github.com/bnb-chain/tss-lib/v2/tss"
	"github.com/roster90/tss-lib-test/mpc/handler"
	"github.com/roster90/tss-lib-test/mpc/tss"
	"github.com/roster90/tss-lib-test/proto"
)

// GenerateShares implements the gRPC service method
func (s *MPCServer) GenerateShares(ctx context.Context, req *proto.GenerateSharesRequest) (*proto.GenerateSharesResponse, error) {
	log.Printf("[DEBUG] GenerateShares called for party %s (ID: %s, Index: %d)", s.partyID.Moniker, s.partyID.Id, s.partyID.Index)

	// Check if party is initialized
	if s.party == nil {
		// Initialize party and channels
		party, chOut, endCh, errCh := tss.InitLocalParty(s.params)
		s.party = party
		s.chOut = chOut
		s.endCh = endCh

		// Start error handler
		go handler.ProcessErrors(errCh)
		log.Printf("[DEBUG] Initialized new party for keygen with params: threshold=%d, partyCount=%d", s.params.Threshold(), len(s.partyIDs))
	}

	// Get the keygen party
	keygenParty := s.party
	log.Printf("[DEBUG] Party type: %T, State: %v", keygenParty, keygenParty)

	// Start message handler for this session
	go handler.ProcessMessages(s.nodeID, s.chOut, s.partyID)
	log.Printf("[DEBUG] Started message handler for node %s", s.nodeID)

	// Start keygen process
	log.Printf("[DEBUG] Starting keygen process for party %s", s.partyID)
	if err := keygenParty.Start(); err != nil {
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
	case err := <-s.endCh:
		log.Printf("[ERROR] Keygen failed: %v", err)
		return nil, fmt.Errorf("keygen failed: %v", err)
	case data := <-s.endCh:
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

	// Initialize party if not already initialized
	if s.party == nil {
		log.Printf("[DEBUG] Initializing party for %s when receiving message", s.nodeID)
		party, chOut, endCh, errCh := tss.InitLocalParty(s.params)
		s.party = party
		s.chOut = chOut
		s.endCh = endCh

		// Start error handler
		go handler.ProcessErrors(errCh)
		log.Printf("[DEBUG] Initialized new party for keygen with params: threshold=%d, partyCount=%d", s.params.Threshold(), len(s.partyIDs))

		// Start keygen process for this node
		log.Printf("[DEBUG] Starting keygen process for node %s", s.nodeID)
		if err := s.party.Start(); err != nil {
			log.Printf("[ERROR] Failed to start keygen for node %s: %v", s.nodeID, err)
			return nil, fmt.Errorf("failed to start keygen: %v", err)
		}
		log.Printf("[DEBUG] Started keygen process for node %s", s.nodeID)
	}

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

	// Get current round and message round
	currentRound := getCurrentRound(s.party)
	messageRound := getMessageRound(msg)
	log.Printf("[DEBUG] Current round: %d, Message round: %d", currentRound, messageRound)

	// Validate round number
	if messageRound < currentRound {
		log.Printf("[WARN] Ignoring message from round %d while in round %d", messageRound, currentRound)
		return &proto.MessageResponse{}, nil
	}

	log.Printf("[DEBUG] Successfully parsed message: Type=%T, From=%s, To=%v",
		msg, msg.GetFrom().Moniker, msg.GetTo())

	// Process the message
	ok, err := s.party.Update(msg)
	if !ok {
		log.Printf("[ERROR] Failed to update party: %v", err)
		return nil, fmt.Errorf("failed to update party: %v", err)
	}

	log.Printf("[DEBUG] Successfully processed message from %s", req.From)
	return &proto.MessageResponse{}, nil
}

// getCurrentRound returns the current round number of the party
func getCurrentRound(party tssLib.Party) int {
	switch p := party.(type) {
	case *keygen.LocalParty:
		// Get round from party's type
		partyType := reflect.TypeOf(p).String()
		log.Printf("[DEBUG] Party type: %s", partyType)
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
	default:
		log.Printf("[DEBUG] Unknown party type: %T", party)
		return 0
	}
}

// getMessageRound extracts the round number from the message type
func getMessageRound(msg tssLib.Message) int {
	// Get message type
	msgType := reflect.TypeOf(msg).String()
	log.Printf("[DEBUG] Full message type: %s", msgType)

	// Extract round from message type
	switch {
	case strings.Contains(msgType, "KGRound1Message"):
		log.Printf("[DEBUG] Detected Round 1 message")
		return 1
	case strings.Contains(msgType, "KGRound2Message"):
		log.Printf("[DEBUG] Detected Round 2 message")
		return 2
	case strings.Contains(msgType, "KGRound3Message"):
		log.Printf("[DEBUG] Detected Round 3 message")
		return 3
	default:
		log.Printf("[DEBUG] Unknown message type: %s", msgType)
		return 0
	}
}
