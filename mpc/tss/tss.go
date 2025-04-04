package tss

import (
	"log"
	"math/big"

	"github.com/bnb-chain/tss-lib/v2/ecdsa/keygen"
	tssLib "github.com/bnb-chain/tss-lib/v2/tss"
)

// InitPartyIDs initializes PartyIDs and PeerContext
func InitPartyIDs(nodeID string) (*tssLib.PartyID, *tssLib.PartyID, *tssLib.PeerContext, []*tssLib.PartyID) {
	log.Printf("[DEBUG] Initializing PartyIDs for node %s", nodeID)

	// Create party IDs with fixed indices
	var myPartyID, otherPartyID *tssLib.PartyID
	var partyIDs []*tssLib.PartyID

	if nodeID == "nodeA" {
		myPartyID = tssLib.NewPartyID("1", "nodeA", big.NewInt(1))
		otherPartyID = tssLib.NewPartyID("2", "nodeB", big.NewInt(2))
		partyIDs = []*tssLib.PartyID{myPartyID, otherPartyID}
	} else if nodeID == "nodeB" {
		myPartyID = tssLib.NewPartyID("2", "nodeB", big.NewInt(2))
		otherPartyID = tssLib.NewPartyID("1", "nodeA", big.NewInt(1))
		partyIDs = []*tssLib.PartyID{otherPartyID, myPartyID}
	} else {
		log.Fatalf("[ERROR] Unknown node ID: %s", nodeID)
	}

	// Log party IDs before sorting
	log.Printf("[DEBUG] PartyIDs before sort:")
	for _, pid := range partyIDs {
		log.Printf("[DEBUG] PartyID: ID=%s, Moniker=%s, Index=%d", pid.Id, pid.Moniker, pid.Index)
	}

	// Sort party IDs by ID
	partyIDs = tssLib.SortPartyIDs(partyIDs)

	// Log party IDs after sorting
	log.Printf("[DEBUG] PartyIDs after sort:")
	for _, pid := range partyIDs {
		log.Printf("[DEBUG] PartyID: ID=%s, Moniker=%s, Index=%d", pid.Id, pid.Moniker, pid.Index)
	}

	// Create peer context
	peerCtx := tssLib.NewPeerContext(partyIDs)
	log.Printf("[DEBUG] Created PeerContext with %d parties", len(partyIDs))

	log.Printf("[DEBUG] Found PartyID for node %s: ID=%s, Moniker=%s, Index=%d",
		nodeID, myPartyID.Id, myPartyID.Moniker, myPartyID.Index)
	return myPartyID, otherPartyID, peerCtx, partyIDs
}

// InitLocalParty initializes the LocalParty and channels
func InitLocalParty(params *tssLib.Parameters) (tssLib.Party, chan tssLib.Message, chan *keygen.LocalPartySaveData, chan error) {
	log.Printf("[DEBUG] Initializing LocalParty with params: threshold=%d, parties=%d", params.Threshold(), params.PartyCount())

	chOut := make(chan tssLib.Message, 2000)
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
