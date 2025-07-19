package main

import (
	"fmt"
	"log"
	"time"

	"github.com/yourusername/distributed-cache/pkg/types"
)

func main() {
	fmt.Println("Testing basic cache dump functionality...")

	// Create a new cache dump
	dump := types.NewCacheDump("node-001")
	
	// Add some sample entries
	entry1 := &types.DumpEntry{
		Key:         "user:123",
		Value:       []byte(`{"name":"John","age":30}`),
		CreatedAt:   time.Now(),
		AccessedAt:  time.Now(),
		AccessCount: 5,
		ExpiresAt:   time.Now().Add(1 * time.Hour),
	}
	
	entry2 := &types.DumpEntry{
		Key:         "session:abc",
		Value:       []byte("session-data-here"),
		CreatedAt:   time.Now().Add(-10 * time.Minute),
		AccessedAt:  time.Now(),
		AccessCount: 1,
		// No expiration
	}
	
	dump.AddEntry(entry1)
	dump.AddEntry(entry2)
	
	// Serialize to JSON
	jsonData, err := dump.ToJSON()
	if err != nil {
		log.Fatalf("Failed to serialize dump: %v", err)
	}
	
	fmt.Printf("Cache dump JSON:\n%s\n", string(jsonData))
	
	// Test deserialization
	restored, err := types.FromJSON(jsonData)
	if err != nil {
		log.Fatalf("Failed to deserialize dump: %v", err)
	}
	
	fmt.Printf("\nRestored dump metadata:\n")
	fmt.Printf("  Node ID: %s\n", restored.NodeID)
	fmt.Printf("  Total entries: %d\n", restored.Metadata.TotalEntries)
	fmt.Printf("  Total size: %d bytes\n", restored.Metadata.TotalSize)
	fmt.Printf("  Max TTL: %d seconds\n", restored.Metadata.MaxTTL)
	
	fmt.Println("\n✅ Basic dump functionality working!")
}
