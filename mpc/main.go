package main

import (
	"context"
	"fmt"
	"log"
	"math/big"
	"net"
	"os"
	"sync"
	"time"

	"github.com/bnb-chain/tss-lib/v2/ecdsa/keygen"
	tss "github.com/bnb-chain/tss-lib/v2/tss"
	pd "github.com/roster90/tss-lib-test/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection"
)

type MPCServer struct {
	pd.UnimplementedMPCServiceServer
	partyID  *tss.PartyID
	partyIDs []*tss.PartyID
	params   *tss.Parameters
	nodeID   string
	peers    map[string]string
	party    tss.Party
	chOut    chan tss.Message
	start    sync.Once
}

var (
	wg         sync.WaitGroup
	nodesReady sync.WaitGroup
)

func main() {
	peers := map[string]string{
		"nodeA": "localhost:50051",
		"nodeB": "localhost:50052",
	}

	nodesReady.Add(2)
	wg.Add(2)
	go startNode("nodeA", ":50051", peers)
	go startNode("nodeB", ":50052", peers)

	wg.Wait()
}

func startNode(nodeID, port string, peers map[string]string) {
	lis, err := net.Listen("tcp", port)
	if err != nil {
		log.Fatalf("Failed to listen on %s: %v", port, err)
	}

	s := grpc.NewServer()
	server := &MPCServer{nodeID: nodeID, peers: peers}
	pd.RegisterMPCServiceServer(s, server)
	reflection.Register(s)

	log.Printf("MPC %s on %s (PID: %d)", nodeID, port, os.Getpid())

	wg.Add(1)
	go func() {
		defer wg.Done()

		partyID, peerCtx, partyIDs := initPartyIDs(nodeID)
		server.partyID = partyID
		server.partyIDs = partyIDs
		server.params = tss.NewParameters(tss.S256(), peerCtx, server.partyID, 2, 1)
		if server.params == nil {
			log.Fatalf("Failed to initialize parameters for node %s", nodeID)
		}

		party, chOut, endCh, errCh := initLocalParty(server.params)
		server.party = party
		server.chOut = chOut

		nodesReady.Done()
		nodesReady.Wait()

		for peer, addr := range peers {
			if peer != nodeID {
				if !waitForConnection(addr, 10*time.Second) {
					log.Fatalf("Failed to connect to %s from %s", peer, nodeID)
				}
				log.Printf("Successfully connected to %s from %s", peer, nodeID)
			}
		}

		server.start.Do(func() {
			log.Printf("Starting keygen and message handler for %s", nodeID)
			go startKeygen(server, endCh, errCh)
			go handleMessages(nodeID, peers, server.chOut, server.partyID)
			go handleErrorMessages(server, errCh)
		})

		select {
		case err := <-errCh:
			log.Printf("Keygen failed for %s: %v", nodeID, err)
		case data := <-endCh:
			log.Printf("Keygen completed for %s: PublicKey=%s,%s", nodeID, data.ECDSAPub.X().String(), data.ECDSAPub.Y().String())
		case <-time.After(600 * time.Second):
			log.Printf("Keygen timed out for %s after 600 seconds", nodeID)
		}
	}()

	if err := s.Serve(lis); err != nil {
		log.Fatalf("Failed to serve %s: %v", nodeID, err)
	}
}

func waitForConnection(addr string, timeout time.Duration) bool {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	for {
		conn, err := grpc.DialContext(ctx, addr, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
		if err == nil {
			conn.Close()
			return true
		}
		if ctx.Err() != nil {
			return false
		}
		log.Printf("Waiting for connection to %s...", addr)
		time.Sleep(500 * time.Millisecond)
	}
}

func startKeygen(server *MPCServer, endCh chan *keygen.LocalPartySaveData, errCh chan error) {
	if err := server.party.Start(); err != nil {
		errCh <- fmt.Errorf("keygen failed for %s: %v", server.nodeID, err)
		return
	}
	log.Printf("Keygen started for %s", server.nodeID)
}

func handleMessages(nodeID string, peers map[string]string, chOut chan tss.Message, fromPartyID *tss.PartyID) {
	ctx := context.Background()
	idToMoniker := map[string]string{"1": "nodeA", "2": "nodeB"}
	monikerToId := map[string]string{"nodeA": "1", "nodeB": "2"}

	for msg := range chOut {
		recipients := msg.GetTo()
		isBroadcast := len(recipients) == 0
		if isBroadcast {
			log.Printf("Broadcasting from %s to all peers", nodeID)
		}

		targets := recipients
		if isBroadcast {
			targets = make([]*tss.PartyID, 0, len(peers)-1)
			for peerMoniker := range peers {
				if peerMoniker != nodeID {
					id := monikerToId[peerMoniker]
					targets = append(targets, tss.NewPartyID(id, peerMoniker, big.NewInt(0)))
				}
			}
		}

		msgBytes, _, err := msg.WireBytes()
		if err != nil {
			log.Printf("Failed to serialize message from %s: %v", nodeID, err)
			continue
		}
		msgType := msg.Type()
		log.Printf("Message prepared from %s: Type=%s, IsBroadcast=%v", nodeID, msgType, isBroadcast)

		for _, peerID := range targets {
			moniker := idToMoniker[peerID.Id]
			if moniker == "" {
				log.Printf("Unknown peer ID: %s", peerID.Id)
				continue
			}
			peerAddr := peers[moniker]
			if err := sendMessageToPeer(ctx, nodeID, moniker, peerAddr, msgBytes); err == nil {
				log.Printf("Sent message from %s to %s (Type: %s)", nodeID, moniker, msgType)
			} else {
				log.Printf("Failed to send message from %s to %s: %v", nodeID, moniker, err)
			}
		}
	}
}

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

func handleErrorMessages(server *MPCServer, errCh chan error) {
	for err := range errCh {
		log.Printf("Error received in errCh for %s: %v", server.nodeID, err)
	}
}

func initPartyIDs(nodeID string) (*tss.PartyID, *tss.PeerContext, []*tss.PartyID) {
	partyIDs := []*tss.PartyID{
		tss.NewPartyID("1", "nodeA", big.NewInt(1)),
		tss.NewPartyID("2", "nodeB", big.NewInt(2)),
	}
	partyIDs = tss.SortPartyIDs(partyIDs)
	peerCtx := tss.NewPeerContext(partyIDs)
	for _, pid := range partyIDs {
		if pid.Moniker == nodeID {
			return pid, peerCtx, partyIDs
		}
	}
	log.Fatalf("Failed to find party ID for node %s", nodeID)
	return nil, nil, nil
}

func initLocalParty(params *tss.Parameters) (tss.Party, chan tss.Message, chan *keygen.LocalPartySaveData, chan error) {
	chOut := make(chan tss.Message, 2000)
	endCh := make(chan *keygen.LocalPartySaveData, 1)
	errCh := make(chan error, 1)
	party := keygen.NewLocalParty(params, chOut, endCh)
	if party == nil {
		log.Fatal("Failed to initialize party")
	}
	return party, chOut, endCh, errCh
}

func (s *MPCServer) ReceiveMessage(ctx context.Context, req *pd.MessageRequest) (*pd.MessageResponse, error) {
	log.Printf("%s received message from %s", s.nodeID, req.From)
	if s.party == nil {
		log.Printf("Party is nil for %s", s.nodeID)
		return &pd.MessageResponse{Success: false}, fmt.Errorf("party not initialized")
	}

	senderPartyID := s.findPartyID(req.From)
	if senderPartyID == nil {
		log.Printf("Invalid sender %s", req.From)
		return &pd.MessageResponse{Success: false}, fmt.Errorf("invalid sender: %s", req.From)
	}

	log.Printf("Sender PartyID: Id=%s, Moniker=%s, Index=%d", senderPartyID.Id, senderPartyID.Moniker, senderPartyID.Index)
	// log.Printf("Raw message bytes from %s: %x", req.From, req.Message)

	msg, err := tss.ParseWireMessage(req.Message, senderPartyID, true)
	if err != nil {
		log.Printf("Failed to parse message from %s: %v", req.From, err)
		return &pd.MessageResponse{Success: false}, fmt.Errorf("parse failed: %v", err)
	}

	round := determineRound(msg.Type())
	log.Printf("Parsed message from %s (Index: %d) - Type: %s, Round: %s", req.From, senderPartyID.Index, msg.Type(), round)

	// Debug trạng thái party trước khi update
	if r, ok := s.party.(interface{ RoundNumber() int }); ok {
		log.Printf("Current round number for %s before update: %d", s.nodeID, r.RoundNumber())
	}

	ok, err := s.party.UpdateFromBytes(req.Message, senderPartyID, true)
	log.Printf("UpdateFromBytes for %s from %s: Success=%v, Error=%v", s.nodeID, req.From, ok, err)

	if !ok {
		log.Printf("Update failed for %s from %s: %v", s.nodeID, req.From, err)
		return &pd.MessageResponse{Success: false}, fmt.Errorf("update failed: %v", err)
	}

	// if err != nil {
	// 	log.Printf("Update error for %s from %s: %v", s.nodeID, req.From, err)
	// 	return &pd.MessageResponse{Success: false}, fmt.Errorf("update error: %v", err)
	// }

	// Debug trạng thái party sau khi update
	if r, ok := s.party.(interface{ RoundNumber() int }); ok {
		log.Printf("Current round number for %s after update: %d", s.nodeID, r.RoundNumber())
	}

	log.Printf("Successfully updated party state for %s from %s", s.nodeID, req.From)
	return &pd.MessageResponse{Success: true}, nil
}

func (s *MPCServer) findPartyID(moniker string) *tss.PartyID {
	for _, pid := range s.partyIDs {
		if pid.Moniker == moniker {
			return pid
		}
	}
	return nil
}

func determineRound(msgType string) string {
	switch msgType {
	case "binance.tsslib.ecdsa.keygen.KGRound1Message":
		return "1"
	case "binance.tsslib.ecdsa.keygen.KGRound2Message1":
		return "2 (Part 1)"
	case "binance.tsslib.ecdsa.keygen.KGRound2Message2":
		return "2 (Part 2)"
	case "binance.tsslib.ecdsa.keygen.KGRound3Message":
		return "3"
	default:
		return "unknown"
	}
}

func (s *MPCServer) GenerateShares(ctx context.Context, req *pd.GenerateSharesRequest) (*pd.GenerateSharesResponse, error) {
	return nil, fmt.Errorf("not implemented")
}
