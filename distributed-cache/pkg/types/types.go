package types

import (
	"context"
	"time"
)

// CacheEntry represents a single cache entry
type CacheEntry struct {
	Key        string    `json:"key"`
	Value      []byte    `json:"value"`
	ExpiresAt  time.Time `json:"expires_at"`
	CreatedAt  time.Time `json:"created_at"`
	AccessedAt time.Time `json:"accessed_at"`
	AccessCount int64    `json:"access_count"`
}

// IsExpired checks if the cache entry has expired
func (e *CacheEntry) IsExpired() bool {
	if e.ExpiresAt.IsZero() {
		return false
	}
	return time.Now().After(e.ExpiresAt)
}

// Cache defines the interface for cache operations
type Cache interface {
	Get(ctx context.Context, key string) (*CacheEntry, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
	Exists(ctx context.Context, key string) (bool, error)
	Size() int64
	Clear() error
}

// CacheStats holds cache statistics
type CacheStats struct {
	Hits      int64 `json:"hits"`
	Misses    int64 `json:"misses"`
	Size      int64 `json:"size"`
	MaxSize   int64 `json:"max_size"`
	Evictions int64 `json:"evictions"`
}

// EvictionPolicy defines how entries should be evicted
type EvictionPolicy int

const (
	LRU EvictionPolicy = iota
	LFU
	FIFO
)

// NodeStatus represents the status of a cluster node
type NodeStatus int

const (
	NodeStatusUnknown NodeStatus = iota
	NodeStatusAlive
	NodeStatusSuspected
	NodeStatusDead
)

// Node represents a cluster node
type Node struct {
	ID       string     `json:"id"`
	Address  string     `json:"address"`
	Port     int        `json:"port"`
	Status   NodeStatus `json:"status"`
	LastSeen time.Time  `json:"last_seen"`
}
