package server

import (
	"log"
	"net"
	"time"

	"github.com/bnb-chain/tss-lib/v2/ecdsa/keygen"
	tssLib "github.com/bnb-chain/tss-lib/v2/tss"
	"github.com/roster90/tss-lib-test/mpc/tss"
	"github.com/roster90/tss-lib-test/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

// Session defines the session type
type Session struct {
	party          tssLib.Party
	chOut          chan tssLib.Message
	endCh          chan *keygen.LocalPartySaveData
	startTime      time.Time
	futureMessages map[int][]tssLib.Message
}

// MPCServer defines the gRPC service
type MPCServer struct {
	proto.UnimplementedMPCServiceServer
	nodeID   string
	partyID  *tssLib.PartyID
	partyIDs []*tssLib.PartyID
	params   *tssLib.Parameters
}

func NewMPCServer(nodeID string, partyID *tssLib.PartyID, partyIDs []*tssLib.PartyID, params *tssLib.Parameters) *MPCServer {
	return &MPCServer{
		nodeID:   nodeID,
		partyID:  partyID,
		partyIDs: partyIDs,
		params:   params,
	}
}

// StartNode initializes and starts a node
func StartNode(nodeID, port string) {
	log.Printf("[DEBUG] Starting node %s on port %s", nodeID, port)

	lis, err := net.Listen("tcp", port)
	if err != nil {
		log.Printf("[ERROR] Failed to listen on %s: %v", port, err)
		return
	}

	s := grpc.NewServer()
	server := &MPCServer{
		nodeID: nodeID,
	}
	proto.RegisterMPCServiceServer(s, server)
	reflection.Register(s)

	log.Printf("[DEBUG] Initializing party for node %s", nodeID)
	myPartyID, peerCtx, partyIDs := tss.InitPartyIDs(nodeID)

	// Set up server with party information
	server.partyID = myPartyID
	server.partyIDs = partyIDs
	server.params = tssLib.NewParameters(tssLib.S256(), peerCtx, myPartyID, 2, 1) // 2 nodes, threshold 1

	// Log party setup details
	log.Printf("[DEBUG] Party setup for %s: ID=%s, Index=%d", nodeID, myPartyID.Id, myPartyID.Index)
	for _, pid := range partyIDs {
		log.Printf("[DEBUG] Available party: ID=%s, Moniker=%s, Index=%d", pid.Id, pid.Moniker, pid.Index)
	}

	// Start serving in a goroutine
	go func() {
		log.Printf("[DEBUG] Starting gRPC server for %s on %s", nodeID, port)
		if err := s.Serve(lis); err != nil {
			log.Printf("[ERROR] Failed to serve %s: %v", nodeID, err)
			return
		}
	}()

	// Keep the node running
	select {}
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
