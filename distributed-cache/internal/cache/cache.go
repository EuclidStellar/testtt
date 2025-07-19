package cache

import (
	"context"
	"fmt"
	"sync"
	"time"

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