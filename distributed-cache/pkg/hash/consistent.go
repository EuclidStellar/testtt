package hash

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"sync"
)

// ConsistentHash implements consistent hashing with virtual nodes
type ConsistentHash struct {
	mu           sync.RWMutex
	replicas     int                // Number of virtual nodes per physical node
	keys         []uint32           // Sorted ring positions
	hashMap      map[uint32]string  // Maps hash positions to node IDs
	nodes        map[string]bool    // Active nodes
}

// NewConsistentHash creates a new consistent hash ring
func NewConsistentHash(replicas int) *ConsistentHash {
	return &ConsistentHash{
		replicas: replicas,
		hashMap:  make(map[uint32]string),
		nodes:    make(map[string]bool),
	}
}

// hash generates a hash for the given key
func (ch *ConsistentHash) hash(key string) uint32 {
	h := sha256.Sum256([]byte(key))
	return uint32(h[0])<<24 | uint32(h[1])<<16 | uint32(h[2])<<8 | uint32(h[3])
}

// AddNode adds a node to the hash ring
func (ch *ConsistentHash) AddNode(nodeID string) {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	
	if ch.nodes[nodeID] {
		return // Node already exists
	}
	
	ch.nodes[nodeID] = true
	
	// Add virtual nodes
	for i := 0; i < ch.replicas; i++ {
		virtualKey := fmt.Sprintf("%s:%d", nodeID, i)
		hash := ch.hash(virtualKey)
		ch.keys = append(ch.keys, hash)
		ch.hashMap[hash] = nodeID
	}
	
	sort.Slice(ch.keys, func(i, j int) bool {
		return ch.keys[i] < ch.keys[j]
	})
}

// RemoveNode removes a node from the hash ring
func (ch *ConsistentHash) RemoveNode(nodeID string) {
	ch.mu.Lock()
	defer ch.mu.Unlock()
	
	if !ch.nodes[nodeID] {
		return // Node doesn't exist
	}
	
	delete(ch.nodes, nodeID)
	
	// Remove virtual nodes
	for i := 0; i < ch.replicas; i++ {
		virtualKey := fmt.Sprintf("%s:%d", nodeID, i)
		hash := ch.hash(virtualKey)
		delete(ch.hashMap, hash)
		
		// Remove from keys slice
		for j, key := range ch.keys {
			if key == hash {
				ch.keys = append(ch.keys[:j], ch.keys[j+1:]...)
				break
			}
		}
	}
}

// GetNode returns the node responsible for the given key
func (ch *ConsistentHash) GetNode(key string) (string, bool) {
	ch.mu.RLock()
	defer ch.mu.RUnlock()
	
	if len(ch.keys) == 0 {
		return "", false
	}
	
	hash := ch.hash(key)
	
	// Binary search for the first node clockwise from the hash
	idx := sort.Search(len(ch.keys), func(i int) bool {
		return ch.keys[i] >= hash
	})
	
	// Wrap around if we're past the last node
	if idx == len(ch.keys) {
		idx = 0
	}
	
	nodeID := ch.hashMap[ch.keys[idx]]
	return nodeID, true
}

// GetNodes returns multiple nodes for replication (including replicas)
func (ch *ConsistentHash) GetNodes(key string, count int) []string {
	ch.mu.RLock()
	defer ch.mu.RUnlock()
	
	if len(ch.keys) == 0 || count <= 0 {
		return nil
	}
	
	hash := ch.hash(key)
	seen := make(map[string]bool)
	var result []string
	
	// Find starting position
	idx := sort.Search(len(ch.keys), func(i int) bool {
		return ch.keys[i] >= hash
	})
	
	// Collect unique nodes clockwise
	for len(result) < count && len(seen) < len(ch.nodes) {
		if idx >= len(ch.keys) {
			idx = 0
		}
		
		nodeID := ch.hashMap[ch.keys[idx]]
		if !seen[nodeID] {
			seen[nodeID] = true
			result = append(result, nodeID)
		}
		idx++
	}
	
	return result
}

// GetAllNodes returns all active nodes
func (ch *ConsistentHash) GetAllNodes() []string {
	ch.mu.RLock()
	defer ch.mu.RUnlock()
	
	var nodes []string
	for nodeID := range ch.nodes {
		nodes = append(nodes, nodeID)
	}
	return nodes
}

// Stats returns statistics about the hash ring
func (ch *ConsistentHash) Stats() map[string]interface{} {
	ch.mu.RLock()
	defer ch.mu.RUnlock()
	
	return map[string]interface{}{
		"nodes":         len(ch.nodes),
		"virtual_nodes": len(ch.keys),
		"replicas":      ch.replicas,
	}
}
