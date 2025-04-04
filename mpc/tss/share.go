package tss

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
)

// ShareData represents the data to be saved for each node
type ShareData struct {
	NodeID    string `json:"node_id"`
	Share     string `json:"share"`
	PublicKey string `json:"public_key"`
}

// SaveShare saves the share data to a JSON file in the node's folder
func SaveShare(nodeID, share, publicKey string) error {
	// Create data structure
	data := ShareData{
		NodeID:    nodeID,
		Share:     share,
		PublicKey: publicKey,
	}

	// Create node-specific directory
	dir := fmt.Sprintf("shares/%s", nodeID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %v", err)
	}

	// Create file path
	filePath := filepath.Join(dir, "share.json")

	// Convert to JSON
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %v", err)
	}

	// Write to file
	if err := os.WriteFile(filePath, jsonData, 0644); err != nil {
		return fmt.Errorf("failed to write file: %v", err)
	}

	log.Printf("[DEBUG] Saved share data for node %s to %s", nodeID, filePath)
	return nil
}
