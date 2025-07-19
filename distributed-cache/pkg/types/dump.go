package types

import (
	"encoding/json"
	"fmt"
	"time"
)

// CacheDump represents a serializable snapshot of the cache state
type CacheDump struct {
	Timestamp time.Time                `json:"timestamp"`
	Version   string                   `json:"version"`
	NodeID    string                   `json:"node_id"`
	Entries   map[string]*DumpEntry    `json:"entries"`
	Metadata  *DumpMetadata            `json:"metadata"`
}

// DumpEntry represents a cache entry in the dump
type DumpEntry struct {
	Key         string    `json:"key"`
	Value       []byte    `json:"value"`
	ExpiresAt   time.Time `json:"expires_at,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	AccessedAt  time.Time `json:"accessed_at"`
	AccessCount int64     `json:"access_count"`
}

// DumpMetadata contains metadata about the cache dump
type DumpMetadata struct {
	TotalEntries int64 `json:"total_entries"`
	TotalSize    int64 `json:"total_size_bytes"`
	MaxTTL       int64 `json:"max_ttl_seconds"`
}

// NewCacheDump creates a new cache dump instance
func NewCacheDump(nodeID string) *CacheDump {
	return &CacheDump{
		Timestamp: time.Now(),
		Version:   "1.0",
		NodeID:    nodeID,
		Entries:   make(map[string]*DumpEntry),
		Metadata:  &DumpMetadata{},
	}
}

// AddEntry adds a cache entry to the dump
func (d *CacheDump) AddEntry(entry *DumpEntry) {
	if d.Entries == nil {
		d.Entries = make(map[string]*DumpEntry)
	}
	d.Entries[entry.Key] = entry
	d.updateMetadata()
}

// ToJSON serializes the cache dump to JSON
func (d *CacheDump) ToJSON() ([]byte, error) {
	return json.MarshalIndent(d, "", "  ")
}

// FromJSON deserializes a cache dump from JSON
func FromJSON(data []byte) (*CacheDump, error) {
	var dump CacheDump
	if err := json.Unmarshal(data, &dump); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cache dump: %w", err)
	}
	return &dump, nil
}

// updateMetadata recalculates the metadata based on current entries
func (d *CacheDump) updateMetadata() {
	if d.Metadata == nil {
		d.Metadata = &DumpMetadata{}
	}
	
	d.Metadata.TotalEntries = int64(len(d.Entries))
	d.Metadata.TotalSize = 0
	d.Metadata.MaxTTL = 0
	
	for _, entry := range d.Entries {
		// Calculate entry size (key + value)
		entrySize := int64(len(entry.Key) + len(entry.Value))
		d.Metadata.TotalSize += entrySize
		
		// Calculate TTL if entry has expiration
		if !entry.ExpiresAt.IsZero() {
			ttl := int64(time.Until(entry.ExpiresAt).Seconds())
			if ttl > d.Metadata.MaxTTL {
				d.Metadata.MaxTTL = ttl
			}
		}
	}
}

// ConvertCacheEntry converts a types.CacheEntry to a DumpEntry
func ConvertCacheEntry(entry *CacheEntry) *DumpEntry {
	return &DumpEntry{
		Key:         entry.Key,
		Value:       entry.Value,
		ExpiresAt:   entry.ExpiresAt,
		CreatedAt:   entry.CreatedAt,
		AccessedAt:  entry.AccessedAt,
		AccessCount: entry.AccessCount,
	}
}

// ValidateDump performs basic validation on a cache dump
func (d *CacheDump) ValidateDump() error {
	if d.NodeID == "" {
		return fmt.Errorf("node ID cannot be empty")
	}
	
	if d.Entries == nil {
		return fmt.Errorf("entries map cannot be nil")
	}
	
	// Validate that entry keys match map keys
	for key, entry := range d.Entries {
		if entry.Key != key {
			return fmt.Errorf("entry key mismatch: map key '%s' != entry key '%s'", key, entry.Key)
		}
	}
	
	return nil
}
