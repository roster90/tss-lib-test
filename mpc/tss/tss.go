package tss

import (
	"log"
	"math/big"

	"github.com/bnb-chain/tss-lib/v2/ecdsa/keygen"
	tssLib "github.com/bnb-chain/tss-lib/v2/tss"
)

// InitPartyIDs initializes PartyIDs and PeerContext for a single node
func InitPartyIDs(nodeID string) (*tssLib.PartyID, *tssLib.PeerContext, []*tssLib.PartyID) {
	log.Printf("[DEBUG] Initializing PartyIDs for node %s", nodeID)

	// Create party IDs for both nodes
	nodeA := tssLib.NewPartyID("1", "nodeA", big.NewInt(1))
	nodeB := tssLib.NewPartyID("2", "nodeB", big.NewInt(2))

	// Set up based on current node
	var myPartyID *tssLib.PartyID
	var partyIDs []*tssLib.PartyID

	if nodeID == "nodeA" {
		myPartyID = nodeA
		partyIDs = []*tssLib.PartyID{nodeA, nodeB}
	} else if nodeID == "nodeB" {
		myPartyID = nodeB
		partyIDs = []*tssLib.PartyID{nodeA, nodeB}
	} else {
		log.Fatalf("[ERROR] Unknown node ID: %s", nodeID)
	}

	// Log party IDs
	log.Printf("[DEBUG] PartyID: ID=%s, Moniker=%s, Index=%d", myPartyID.Id, myPartyID.Moniker, myPartyID.Index)

	// Create peer context
	peerCtx := tssLib.NewPeerContext(partyIDs)
	log.Printf("[DEBUG] Created PeerContext with %d parties", len(partyIDs))

	// Sort party IDs
	partyIDs = tssLib.SortPartyIDs(partyIDs)

	log.Printf("[DEBUG] Found PartyID for node %s: ID=%s, Moniker=%s, Index=%d",
		nodeID, myPartyID.Id, myPartyID.Moniker, myPartyID.Index)

	return myPartyID, peerCtx, partyIDs

}

// InitLocalParty initializes the LocalParty and channels
func InitLocalParty(params *tssLib.Parameters) (tssLib.Party, chan tssLib.Message, chan *keygen.LocalPartySaveData, chan error) {

	log.Printf("[DEBUG] Initializing LocalParty with params: threshold=%d, parties=%d", params.Threshold(), params.PartyCount())

	chOut := make(chan tssLib.Message, 20000)
	endCh := make(chan *keygen.LocalPartySaveData, 1)
	errCh := make(chan error, 1)

	party := keygen.NewLocalParty(params, chOut, endCh)

	if party == nil {
		log.Fatal("[ERROR] Failed to initialize party")
	}

	pid := party.PartyID()
	log.Printf("[DEBUG] Created LocalParty: ID=%s, Moniker=%s, Index=%d", pid.Id, pid.Moniker, pid.Index)

	return party, chOut, endCh, errCh
}
