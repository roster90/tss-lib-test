package server

import (
	"context"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/bnb-chain/tss-lib/v2/ecdsa/keygen"
	tssLib "github.com/bnb-chain/tss-lib/v2/tss"
	"github.com/roster90/tss-lib-test/mpc/handler"
	"github.com/roster90/tss-lib-test/mpc/tss"
	"github.com/roster90/tss-lib-test/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

// MPCServer defines the gRPC service
type MPCServer struct {
	proto.UnimplementedMPCServiceServer
	partyID  *tssLib.PartyID
	partyIDs []*tssLib.PartyID
	params   *tssLib.Parameters
	nodeID   string
	peers    map[string]string
	party    tssLib.Party
	chOut    chan tssLib.Message
	endCh    chan *keygen.LocalPartySaveData
	start    sync.Once
}

var (
	wg         sync.WaitGroup
	nodesReady sync.WaitGroup // Synchronize all nodes
)

// StartNode initializes and starts a node
func StartNode(nodeID, port string, peers map[string]string) {
	log.Printf("[DEBUG] Starting node %s on port %s", nodeID, port)

	// Initialize nodesReady counter based on number of peers plus self
	nodesReady.Add(len(peers) + 1)
	log.Printf("[DEBUG] Waiting for %d nodes to be ready", len(peers)+1)

	lis, err := net.Listen("tcp", port)
	if err != nil {
		log.Printf("[ERROR] Failed to listen on %s: %v", port, err)
		return
	}

	s := grpc.NewServer()
	server := &MPCServer{
		nodeID: nodeID,
		peers:  peers,
		endCh:  make(chan *keygen.LocalPartySaveData, 1),
	}
	proto.RegisterMPCServiceServer(s, server)
	reflection.Register(s)

	log.Printf("[DEBUG] Initializing party for node %s", nodeID)
	myPartyID, _, peerCtx, partyIDs := tss.InitPartyIDs(nodeID)

	// Set up server with party information
	server.partyID = myPartyID
	server.partyIDs = partyIDs
	server.params = tssLib.NewParameters(tssLib.S256(), peerCtx, myPartyID, 2, 1) // 2 nodes, threshold 1

	log.Printf("[DEBUG] Party setup for %s: ID=%s, Index=%d", nodeID, myPartyID.Id, myPartyID.Index)
	for _, pid := range partyIDs {
		log.Printf("[DEBUG] Available party: ID=%s, Moniker=%s, Index=%d", pid.Id, pid.Moniker, pid.Index)
	}

	// Signal that this node is ready
	nodesReady.Done()
	log.Printf("[DEBUG] Node %s is ready", nodeID)

	// Start serving in a goroutine
	go func() {
		log.Printf("[DEBUG] Starting gRPC server for %s", nodeID)
		if err := s.Serve(lis); err != nil {
			log.Printf("[ERROR] Failed to serve %s: %v", nodeID, err)
			return
		}
	}()
}

// GenerateShares implements the gRPC service method
func (s *MPCServer) GenerateShares(ctx context.Context, req *proto.GenerateSharesRequest) (*proto.GenerateSharesResponse, error) {
	log.Printf("[DEBUG] GenerateShares called for party %s", s.partyID)

	// Check if party is initialized
	if s.party == nil {
		// Initialize party and channels
		party, chOut, endCh, errCh := tss.InitLocalParty(s.params)
		s.party = party
		s.chOut = chOut
		s.endCh = endCh

		// Start error handler
		go handler.HandleErrorMessages(errCh)
		log.Printf("[DEBUG] Initialized new party for keygen")
	}

	// Get the keygen party
	keygenParty := s.party
	log.Printf("[DEBUG] Party type: %T", keygenParty)

	// Start message handler for this session
	go handler.HandleMessages(s.nodeID, s.peers, s.chOut, s.partyID)

	// Start keygen process
	log.Printf("[DEBUG] Starting keygen process for party %s", s.partyID)
	if err := keygenParty.Start(); err != nil {
		log.Printf("[ERROR] Failed to start keygen: %v", err)
		return nil, fmt.Errorf("failed to start keygen: %v", err)
	}
	log.Printf("[DEBUG] Started keygen process")

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

		log.Printf("[DEBUG] Received data from endCh: %+v", data)

		// Convert to string format
		pubKeyStr := fmt.Sprintf("%s,%s", data.ECDSAPub.X().String(), data.ECDSAPub.Y().String())
		sharesStr := data.Xi.String()

		log.Printf("[DEBUG] Generated public key: %s", pubKeyStr)
		log.Printf("[DEBUG] Generated share: %s", sharesStr)

		// Save share to JSON file
		if err := tss.SaveShare(s.nodeID, sharesStr, pubKeyStr); err != nil {
			log.Printf("[ERROR] Failed to save share: %v", err)
			return nil, fmt.Errorf("failed to save share: %v", err)
		}

		return &proto.GenerateSharesResponse{
			PublicKey: pubKeyStr,
			Shares:    sharesStr,
		}, nil
	case <-time.After(30 * time.Second):
		log.Printf("[ERROR] Timeout waiting for keygen completion")
		return nil, fmt.Errorf("keygen not completed yet")
	}
}

// ReceiveMessage implements the gRPC service method for receiving messages from other nodes
func (s *MPCServer) ReceiveMessage(ctx context.Context, req *proto.MessageRequest) (*proto.MessageResponse, error) {
	// Initialize party if not already initialized
	if s.party == nil {
		log.Printf("[DEBUG] Initializing party for %s when receiving message", s.nodeID)
		party, chOut, endCh, errCh := tss.InitLocalParty(s.params)
		s.party = party
		s.chOut = chOut
		s.endCh = endCh

		// Start error handler
		go handler.HandleErrorMessages(errCh)
		log.Printf("[DEBUG] Initialized new party for keygen")
	}

	// Find the sender's PartyID
	senderPartyID := findPartyID(req.From, s.partyIDs)
	if senderPartyID == nil {
		return nil, fmt.Errorf("invalid sender: %s", req.From)
	}

	log.Printf("[DEBUG] Processing message from %s (Index: %d)", req.From, senderPartyID.Index)
	log.Printf("[DEBUG] Message content length: %d", len(req.Message))

	// Parse and process the message
	msg, err := tssLib.ParseWireMessage(req.Message, senderPartyID, true)
	if err != nil {
		log.Printf("[ERROR] Failed to parse message: %v", err)
		return nil, fmt.Errorf("failed to parse message: %v", err)
	}

	log.Printf("[DEBUG] Parsed message content type: %T", msg.Content())

	// For KGRound2Message1, ensure it's not broadcast
	if _, ok := msg.Content().(*keygen.KGRound2Message1); ok {
		log.Printf("[DEBUG] Found KGRound2Message1, forcing non-broadcast mode")
		ok, err := s.party.UpdateFromBytes(req.Message, senderPartyID, false)
		if !ok {
			log.Printf("[ERROR] Failed to update party with KGRound2Message1: %v", err)
			return nil, fmt.Errorf("failed to update party: %v", err)
		}
		log.Printf("[DEBUG] Successfully processed KGRound2Message1")
		return &proto.MessageResponse{}, nil
	}

	// For other messages, process normally
	ok, err := s.party.UpdateFromBytes(req.Message, senderPartyID, true)
	if !ok {
		log.Printf("[ERROR] Failed to update party: %v", err)
		return nil, fmt.Errorf("failed to update party: %v", err)
	}

	log.Printf("[DEBUG] Successfully processed message from %s", req.From)
	return &proto.MessageResponse{}, nil
}

// findPartyID looks for the PartyID based on the sender's moniker
func findPartyID(moniker string, partyIDs []*tssLib.PartyID) *tssLib.PartyID {
	// Find the PartyID in the provided partyIDs
	for _, pid := range partyIDs {
		if pid.Moniker == moniker {
			log.Printf("[DEBUG] Found PartyID for %s: ID=%s, Index=%d", moniker, pid.Id, pid.Index)
			return pid
		}
	}

	log.Printf("[ERROR] Unknown moniker: %s", moniker)
	return nil
}
