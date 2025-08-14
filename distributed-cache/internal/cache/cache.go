package cache

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/yourusername/distributed-cache/pkg/hash"
	"github.com/yourusername/distributed-cache/pkg/types"
)

// MemoryStorage implements an in-memory cache with thread safety
type MemoryStorage struct {
	mu         sync.RWMutex
	data       map[string]*types.CacheEntry
	maxSize    int64
	stats      *types.CacheStats
	eviction   types.EvictionPolicy
	accessList *accessList // For LRU implementation
}

// NewMemoryStorage creates a new memory storage instance
func NewMemoryStorage(maxSize int64, evictionPolicy types.EvictionPolicy) *MemoryStorage {
	return &MemoryStorage{
		data:       make(map[string]*types.CacheEntry),
		maxSize:    maxSize,
		eviction:   evictionPolicy,
		stats:      &types.CacheStats{MaxSize: maxSize},
		accessList: newAccessList(),
	}
}

// Get retrieves a value from the cache
func (ms *MemoryStorage) Get(ctx context.Context, key string) (*types.CacheEntry, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	ms.mu.RLock()
	entry, exists := ms.data[key]
	ms.mu.RUnlock()

	if !exists {
		ms.mu.Lock()
		ms.stats.Misses++
		ms.mu.Unlock()
		return nil, fmt.Errorf("key not found: %s", key)
	}

	// Check expiration
	if entry.IsExpired() {
		ms.mu.Lock()
		delete(ms.data, key)
		ms.stats.Misses++
		ms.stats.Size--
		ms.mu.Unlock()
		return nil, fmt.Errorf("key expired: %s", key)
	}

	// Update access information
	ms.mu.Lock()
	entry.AccessedAt = time.Now()
	entry.AccessCount++
	ms.stats.Hits++

	// Update LRU order
	if ms.eviction == types.LRU {
		ms.accessList.moveToFront(key)
	}
	ms.mu.Unlock()

	return entry, nil
}

// Set stores a value in the cache
func (ms *MemoryStorage) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	ms.mu.Lock()
	defer ms.mu.Unlock()

	// Check if we need to evict entries
	if ms.stats.Size >= ms.maxSize {
		if err := ms.evictEntry(); err != nil {
			return fmt.Errorf("failed to evict entry: %w", err)
		}
	}

	now := time.Now()
	var expiresAt time.Time
	if ttl > 0 {
		expiresAt = now.Add(ttl)
	}

	entry := &types.CacheEntry{
		Key:         key,
		Value:       value,
		ExpiresAt:   expiresAt,
		CreatedAt:   now,
		AccessedAt:  now,
		AccessCount: 1,
	}

	// If key already exists, don't increment size
	if _, exists := ms.data[key]; !exists {
		ms.stats.Size++
		if ms.eviction == types.LRU {
			ms.accessList.addToFront(key)
		}
	} else {
		if ms.eviction == types.LRU {
			ms.accessList.moveToFront(key)
		}
	}

	ms.data[key] = entry
	return nil
}

// Delete removes a key from the cache
func (ms *MemoryStorage) Delete(ctx context.Context, key string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	ms.mu.Lock()
	defer ms.mu.Unlock()

	if _, exists := ms.data[key]; exists {
		delete(ms.data, key)
		ms.stats.Size--
		if ms.eviction == types.LRU {
			ms.accessList.remove(key)
		}
	}

	return nil
}

// Exists checks if a key exists in the cache
func (ms *MemoryStorage) Exists(ctx context.Context, key string) (bool, error) {
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	default:
	}

	ms.mu.RLock()
	entry, exists := ms.data[key]
	ms.mu.RUnlock()

	if !exists {
		return false, nil
	}

	return !entry.IsExpired(), nil
}

// Size returns the current number of entries
func (ms *MemoryStorage) Size() int64 {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	return ms.stats.Size
}

// Clear removes all entries from the cache
func (ms *MemoryStorage) Clear() error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	ms.data = make(map[string]*types.CacheEntry)
	ms.stats.Size = 0
	ms.accessList = newAccessList()
	return nil
}

// GetStats returns cache statistics
func (ms *MemoryStorage) GetStats() *types.CacheStats {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	// Return a copy to avoid race conditions
	stats := *ms.stats
	return &stats
}

// evictEntry removes the least recently used entry (must be called with lock held)
func (ms *MemoryStorage) evictEntry() error {
	if ms.stats.Size == 0 {
		return nil
	}

	var keyToEvict string

	switch ms.eviction {
	case types.LRU:
		keyToEvict = ms.accessList.getLRU()
	case types.LFU:
		keyToEvict = ms.findLFU()
	default:
		// FIFO - find oldest entry
		keyToEvict = ms.findOldest()
	}

	if keyToEvict != "" {
		delete(ms.data, keyToEvict)
		ms.stats.Size--
		ms.stats.Evictions++
		if ms.eviction == types.LRU {
			ms.accessList.remove(keyToEvict)
		}
	}

	return nil
}

// findLFU finds the least frequently used key
func (ms *MemoryStorage) findLFU() string {
	var lfu string
	var minCount int64 = -1

	for key, entry := range ms.data {
		if minCount == -1 || entry.AccessCount < minCount {
			minCount = entry.AccessCount
			lfu = key
		}
	}

	return lfu
}

// findOldest finds the oldest entry (FIFO)
func (ms *MemoryStorage) findOldest() string {
	var oldest string
	var oldestTime time.Time

	for key, entry := range ms.data {
		if oldest == "" || entry.CreatedAt.Before(oldestTime) {
			oldest = key
			oldestTime = entry.CreatedAt
		}
	}

	return oldest
}

// StartCleanup starts a background goroutine to clean up expired entries
func (ms *MemoryStorage) StartCleanup(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				ms.cleanupExpired()
			}
		}
	}()
}

// cleanupExpired removes all expired entries
func (ms *MemoryStorage) cleanupExpired() {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	for key, entry := range ms.data {
		if entry.IsExpired() {
			delete(ms.data, key)
			ms.stats.Size--
			if ms.eviction == types.LRU {
				ms.accessList.remove(key)
			}
		}
	}
}

// Additional distributed cache functionality

var (
	ErrNotLeader      = errors.New("not leader")
	ErrQuorumNotMet   = errors.New("quorum not met")
)

// DistributedCache implements a distributed cache with strong consistency
type DistributedCache struct {
	mu              sync.RWMutex
	nodeID          string
	data            map[string]*types.CacheEntry
	maxSize         int64
	currentSize     int64
	evictionPolicy  EvictionStrategy

	// Cluster management
	hashRing        *hash.ConsistentHash
	replicationFactor int

	// Consistency and replication
	isLeader        bool
	peers           map[string]CachePeer

	// Statistics
	stats           *types.CacheStats

	// Monitoring
	metricsCh       chan CacheMetric
}

// CachePeer represents a peer node in the cluster
type CachePeer interface {
	Get(ctx context.Context, key string) (*types.CacheEntry, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
	Ping(ctx context.Context) error
}

// CacheMetric represents a metric event
type CacheMetric struct {
	Type      string
	Key       string
	Value     float64
	Timestamp time.Time
	Labels    map[string]string
}

// CacheConfig holds configuration for the cache
type CacheConfig struct {
	NodeID            string
	MaxSize           int64
	EvictionPolicy    types.EvictionPolicy
	ReplicationFactor int
	HashRingReplicas  int
}

// NewDistributedCache creates a new distributed cache instance
func NewDistributedCache(config *CacheConfig) *DistributedCache {
	evictionStrategy := CreateEvictionStrategy(config.EvictionPolicy, int(config.MaxSize))

	cache := &DistributedCache{
		nodeID:          config.NodeID,
		data:            make(map[string]*types.CacheEntry),
		maxSize:         config.MaxSize,
		evictionPolicy:  evictionStrategy,
		hashRing:        hash.NewConsistentHash(config.HashRingReplicas),
		replicationFactor: config.ReplicationFactor,
		peers:           make(map[string]CachePeer),
		stats: &types.CacheStats{
			MaxSize: config.MaxSize,
		},
		metricsCh: make(chan CacheMetric, 1000),
	}

	// Add self to hash ring
	cache.hashRing.AddNode(config.NodeID)

	return cache
}

// Get retrieves a value from the cache with strong consistency
func (dc *DistributedCache) Get(ctx context.Context, key string) (*types.CacheEntry, error) {
	start := time.Now()
	defer func() {
		dc.recordMetric("cache_operation_duration", "get", time.Since(start).Seconds())
	}()

	// Check if this node should handle the key
	responsibleNodes := dc.hashRing.GetNodes(key, dc.replicationFactor)
	if len(responsibleNodes) == 0 {
		return nil, fmt.Errorf("key not found: %s", key)
	}

	isResponsible := false
	for _, nodeID := range responsibleNodes {
		if nodeID == dc.nodeID {
			isResponsible = true
			break
		}
	}

	if !isResponsible {
		// Forward to responsible node
		return dc.forwardGet(ctx, key, responsibleNodes[0])
	}

	// Perform quorum read for strong consistency
	return dc.quorumGet(ctx, key, responsibleNodes)
}

// Set stores a value in the cache with replication
func (dc *DistributedCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	start := time.Now()
	defer func() {
		dc.recordMetric("cache_operation_duration", "set", time.Since(start).Seconds())
	}()

	if ttl < 0 {
		return fmt.Errorf("invalid TTL")
	}

	// Check if this node should handle the key
	responsibleNodes := dc.hashRing.GetNodes(key, dc.replicationFactor)
	if len(responsibleNodes) == 0 {
		return fmt.Errorf("no responsible nodes found")
	}

	isResponsible := false
	for _, nodeID := range responsibleNodes {
		if nodeID == dc.nodeID {
			isResponsible = true
			break
		}
	}

	if !isResponsible {
		// Forward to responsible node
		return dc.forwardSet(ctx, key, value, ttl, responsibleNodes[0])
	}

	// Perform quorum write for strong consistency
	return dc.quorumSet(ctx, key, value, ttl, responsibleNodes)
}

// localGet retrieves a value from local storage
func (dc *DistributedCache) localGet(key string) (*types.CacheEntry, error) {
	dc.mu.RLock()
	defer dc.mu.RUnlock()

	entry, exists := dc.data[key]
	if !exists {
		atomic.AddInt64(&dc.stats.Misses, 1)
		dc.recordMetric("cache_misses", "", 1)
		return nil, fmt.Errorf("key not found: %s", key)
	}

	// Check if expired
	if entry.IsExpired() {
		dc.mu.RUnlock()
		dc.mu.Lock()
		delete(dc.data, key)
		dc.evictionPolicy.OnDelete(key)
		dc.updateSize(-int64(len(entry.Key) + len(entry.Value)))
		dc.mu.Unlock()
		dc.mu.RLock()

		atomic.AddInt64(&dc.stats.Misses, 1)
		dc.recordMetric("cache_misses", "", 1)
		return nil, fmt.Errorf("key expired: %s", key)
	}

	// Update access information
	entry.AccessedAt = time.Now()
	entry.AccessCount++
	dc.evictionPolicy.OnAccess(key)

	atomic.AddInt64(&dc.stats.Hits, 1)
	dc.recordMetric("cache_hits", "", 1)

	// Return a copy to avoid external modifications
	return &types.CacheEntry{
		Key:         entry.Key,
		Value:       append([]byte(nil), entry.Value...),
		ExpiresAt:   entry.ExpiresAt,
		CreatedAt:   entry.CreatedAt,
		AccessedAt:  entry.AccessedAt,
		AccessCount: entry.AccessCount,
	}, nil
}

// localSet stores a value in local storage
func (dc *DistributedCache) localSet(key string, value []byte, ttl time.Duration) error {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	now := time.Now()
	entrySize := int64(len(key) + len(value))

	// Check if key already exists
	if existingEntry, exists := dc.data[key]; exists {
		oldSize := int64(len(existingEntry.Key) + len(existingEntry.Value))
		dc.updateSize(entrySize - oldSize)

		// Update existing entry
		existingEntry.Value = append([]byte(nil), value...)
		existingEntry.AccessedAt = now
		existingEntry.AccessCount++
		if ttl > 0 {
			existingEntry.ExpiresAt = now.Add(ttl)
		} else {
			existingEntry.ExpiresAt = time.Time{}
		}

		dc.evictionPolicy.OnInsert(key, entrySize)
		return nil
	}

	// Check if we have space
	if dc.currentSize+entrySize > dc.maxSize {
		// Try to evict entries
		for dc.currentSize+entrySize > dc.maxSize {
			evictedKey := dc.evictionPolicy.EvictOldest()
			if evictedKey == "" {
				return fmt.Errorf("cache is full")
			}

			if evictedEntry, exists := dc.data[evictedKey]; exists {
				evictedSize := int64(len(evictedEntry.Key) + len(evictedEntry.Value))
				delete(dc.data, evictedKey)
				dc.updateSize(-evictedSize)
				atomic.AddInt64(&dc.stats.Evictions, 1)
				dc.recordMetric("cache_evictions", "", 1)
			}
		}
	}

	// Create new entry
	entry := &types.CacheEntry{
		Key:         key,
		Value:       append([]byte(nil), value...),
		CreatedAt:   now,
		AccessedAt:  now,
		AccessCount: 1,
	}

	if ttl > 0 {
		entry.ExpiresAt = now.Add(ttl)
	}

	dc.data[key] = entry
	dc.updateSize(entrySize)
	dc.evictionPolicy.OnInsert(key, entrySize)

	return nil
}

// quorumGet performs a quorum read across replicas
func (dc *DistributedCache) quorumGet(ctx context.Context, key string, nodes []string) (*types.CacheEntry, error) {
	quorumSize := len(nodes)/2 + 1
	responses := make(chan *types.CacheEntry, len(nodes))
	errors := make(chan error, len(nodes))

	// Send requests to all replicas
	for _, nodeID := range nodes {
		go func(nodeID string) {
			if nodeID == dc.nodeID {
				// Local read
				entry, err := dc.localGet(key)
				if err != nil {
					errors <- err
					return
				}
				responses <- entry
			} else {
				// Remote read
				peer, exists := dc.peers[nodeID]
				if !exists {
					errors <- fmt.Errorf("peer not found: %s", nodeID)
					return
				}

				entry, err := peer.Get(ctx, key)
				if err != nil {
					errors <- err
					return
				}
				responses <- entry
			}
		}(nodeID)
	}

	// Wait for quorum responses
	successCount := 0
	var latestEntry *types.CacheEntry

	for i := 0; i < len(nodes); i++ {
		select {
		case entry := <-responses:
			successCount++
			if latestEntry == nil || entry.AccessedAt.After(latestEntry.AccessedAt) {
				latestEntry = entry
			}
			if successCount >= quorumSize {
				return latestEntry, nil
			}
		case <-errors:
			// Continue waiting for more responses
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	if successCount < quorumSize {
		return nil, ErrQuorumNotMet
	}

	return latestEntry, nil
}

// quorumSet performs a quorum write across replicas
func (dc *DistributedCache) quorumSet(ctx context.Context, key string, value []byte, ttl time.Duration, nodes []string) error {
	quorumSize := len(nodes)/2 + 1
	responses := make(chan error, len(nodes))

	// Send requests to all replicas
	for _, nodeID := range nodes {
		go func(nodeID string) {
			if nodeID == dc.nodeID {
				// Local write
				responses <- dc.localSet(key, value, ttl)
			} else {
				// Remote write
				peer, exists := dc.peers[nodeID]
				if !exists {
					responses <- fmt.Errorf("peer not found: %s", nodeID)
					return
				}

				responses <- peer.Set(ctx, key, value, ttl)
			}
		}(nodeID)
	}

	// Wait for quorum responses
	successCount := 0
	var lastError error

	for i := 0; i < len(nodes); i++ {
		select {
		case err := <-responses:
			if err == nil {
				successCount++
				if successCount >= quorumSize {
					return nil
				}
			} else {
				lastError = err
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	if successCount < quorumSize {
		if lastError != nil {
			return lastError
		}
		return ErrQuorumNotMet
	}

	return nil
}

// forwardGet forwards a get request to the responsible node
func (dc *DistributedCache) forwardGet(ctx context.Context, key string, nodeID string) (*types.CacheEntry, error) {
	peer, exists := dc.peers[nodeID]
	if !exists {
		return nil, fmt.Errorf("peer not found: %s", nodeID)
	}

	return peer.Get(ctx, key)
}

// forwardSet forwards a set request to the responsible node
func (dc *DistributedCache) forwardSet(ctx context.Context, key string, value []byte, ttl time.Duration, nodeID string) error {
	peer, exists := dc.peers[nodeID]
	if !exists {
		return fmt.Errorf("peer not found: %s", nodeID)
	}

	return peer.Set(ctx, key, value, ttl)
}

// AddPeer adds a peer to the cluster
func (dc *DistributedCache) AddPeer(nodeID string, peer CachePeer) {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	dc.peers[nodeID] = peer
	dc.hashRing.AddNode(nodeID)
}

// RemovePeer removes a peer from the cluster
func (dc *DistributedCache) RemovePeer(nodeID string) {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	delete(dc.peers, nodeID)
	dc.hashRing.RemoveNode(nodeID)
}

// updateSize updates the current size and stats
func (dc *DistributedCache) updateSize(delta int64) {
	dc.currentSize += delta
	atomic.StoreInt64(&dc.stats.Size, dc.currentSize)
}

// recordMetric records a metric for monitoring
func (dc *DistributedCache) recordMetric(metricType, operation string, value float64) {
	metric := CacheMetric{
		Type:      metricType,
		Value:     value,
		Timestamp: time.Now(),
		Labels: map[string]string{
			"node_id":   dc.nodeID,
			"operation": operation,
		},
	}

	select {
	case dc.metricsCh <- metric:
	default:
		// Channel full, drop metric
	}
}

// GetMetricsChan returns the metrics channel for monitoring
func (dc *DistributedCache) GetMetricsChan() <-chan CacheMetric {
	return dc.metricsCh
}