package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/yourusername/distributed-cache/pkg/batch"
	"github.com/yourusername/distributed-cache/pkg/multilevel"
	"github.com/yourusername/distributed-cache/pkg/pool"
	"github.com/yourusername/distributed-cache/pkg/serialization"
	"github.com/yourusername/distributed-cache/pkg/types"
)

// EnhancedCache provides advanced cache features
type EnhancedCache struct {
	storage         types.Cache
	batchProcessor  *batch.BatchProcessor
	serializer      *serialization.SerializationManager
	connectionPool  *pool.PoolManager
	
	// Configuration
	config          *EnhancedCacheConfig
	
	// Statistics
	stats           *EnhancedCacheStats
	mu              sync.RWMutex
}

// EnhancedCacheConfig holds configuration for enhanced features
type EnhancedCacheConfig struct {
	// Basic cache config
	MaxSize           int64
	DefaultTTL        time.Duration
	EvictionPolicy    types.EvictionPolicy
	
	// Batch configuration
	BatchSize         int
	BatchTimeout      time.Duration
	
	// Serialization
	DefaultSerialization serialization.SerializationFormat
	
	// Connection pooling
	EnableConnectionPool bool
	PoolConfig          *pool.PoolConfig
	
	// Multi-level cache
	EnableMultiLevel    bool
	TierConfigs         []multilevel.CacheTier
}

// EnhancedCacheStats tracks enhanced cache statistics
type EnhancedCacheStats struct {
	mu sync.RWMutex
	
	// Basic stats
	BasicStats       *types.CacheStats
	
	// Batch stats
	BatchOperations  int64
	PipelineExecutions int64
	
	// Serialization stats
	SerializationTime  time.Duration
	DeserializationTime time.Duration
	
	// Multi-level stats
	MultiLevelStats  *multilevel.MultiLevelStats
	
	// Connection pool stats
	PoolStats        pool.PoolStats
}

// DefaultEnhancedCacheConfig returns a default configuration
func DefaultEnhancedCacheConfig() *EnhancedCacheConfig {
	return &EnhancedCacheConfig{
		MaxSize:              1000000,
		DefaultTTL:           time.Hour,
		EvictionPolicy:       types.LRU,
		BatchSize:            100,
		BatchTimeout:         5 * time.Second,
		DefaultSerialization: serialization.JSON,
		EnableConnectionPool: true,
		PoolConfig:          pool.DefaultPoolConfig(),
		EnableMultiLevel:    false,
		TierConfigs:         []multilevel.CacheTier{},
	}
}

// NewEnhancedCache creates a new enhanced cache instance
func NewEnhancedCache(config *EnhancedCacheConfig) (*EnhancedCache, error) {
	if config == nil {
		config = DefaultEnhancedCacheConfig()
	}
	
	// Create base storage
	var storage types.Cache
	if config.EnableMultiLevel && len(config.TierConfigs) > 0 {
		multiCache := multilevel.NewMultiLevelCache(config.TierConfigs, nil)
		storage = multiCache
	} else {
		storage = NewMemoryStorage(config.MaxSize, config.EvictionPolicy)
	}
	
	// Create components
	batchProcessor := batch.NewBatchProcessor(storage, config.BatchSize, config.BatchTimeout)
	serializer := serialization.NewSerializationManager(config.DefaultSerialization)
	
	var connectionPool *pool.PoolManager
	if config.EnableConnectionPool {
		connectionPool = pool.NewPoolManager()
	}
	
	cache := &EnhancedCache{
		storage:        storage,
		batchProcessor: batchProcessor,
		serializer:     serializer,
		connectionPool: connectionPool,
		config:         config,
		stats: &EnhancedCacheStats{
			BasicStats: &types.CacheStats{
				MaxSize: config.MaxSize,
			},
		},
	}
	
	return cache, nil
}

// Basic cache operations

// Get retrieves a value from the cache
func (ec *EnhancedCache) Get(ctx context.Context, key string) (*types.CacheEntry, error) {
	return ec.storage.Get(ctx, key)
}

// Set stores a value in the cache
func (ec *EnhancedCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if ttl == 0 {
		ttl = ec.config.DefaultTTL
	}
	return ec.storage.Set(ctx, key, value, ttl)
}

// Delete removes a key from the cache
func (ec *EnhancedCache) Delete(ctx context.Context, key string) error {
	return ec.storage.Delete(ctx, key)
}

// Exists checks if a key exists in the cache
func (ec *EnhancedCache) Exists(ctx context.Context, key string) (bool, error) {
	return ec.storage.Exists(ctx, key)
}

// Size returns the current number of entries
func (ec *EnhancedCache) Size() int64 {
	return ec.storage.Size()
}

// Clear removes all entries from the cache
func (ec *EnhancedCache) Clear() error {
	return ec.storage.Clear()
}

// Enhanced operations with serialization

// SetObject stores a Go object in the cache with automatic serialization
func (ec *EnhancedCache) SetObject(ctx context.Context, key string, obj interface{}, ttl time.Duration, format serialization.SerializationFormat) error {
	start := time.Now()
	defer func() {
		ec.stats.mu.Lock()
		ec.stats.SerializationTime += time.Since(start)
		ec.stats.mu.Unlock()
	}()
	
	cacheValue, err := ec.serializer.SerializeValue(obj, format)
	if err != nil {
		return fmt.Errorf("serialization failed: %w", err)
	}
	
	// Store the serialized cache value as JSON
	data, err := json.Marshal(cacheValue)
	if err != nil {
		return fmt.Errorf("failed to marshal cache value: %w", err)
	}
	
	return ec.Set(ctx, key, data, ttl)
}

// GetObject retrieves and deserializes a Go object from the cache
func (ec *EnhancedCache) GetObject(ctx context.Context, key string, target interface{}) error {
	start := time.Now()
	defer func() {
		ec.stats.mu.Lock()
		ec.stats.DeserializationTime += time.Since(start)
		ec.stats.mu.Unlock()
	}()
	
	entry, err := ec.Get(ctx, key)
	if err != nil {
		return err
	}
	
	// Unmarshal the cache value
	var cacheValue serialization.CacheValue
	if err := json.Unmarshal(entry.Value, &cacheValue); err != nil {
		return fmt.Errorf("failed to unmarshal cache value: %w", err)
	}
	
	return ec.serializer.DeserializeValue(&cacheValue, target)
}

// Batch operations

// Pipeline creates a new pipeline for batch operations
func (ec *EnhancedCache) Pipeline() *batch.Pipeline {
	return batch.NewPipeline()
}

// ExecutePipeline executes a batch pipeline
func (ec *EnhancedCache) ExecutePipeline(ctx context.Context, pipeline *batch.Pipeline) ([]batch.BatchResult, error) {
	ec.stats.mu.Lock()
	ec.stats.PipelineExecutions++
	ec.stats.mu.Unlock()
	
	return pipeline.Execute(ctx, ec.storage)
}

// MGet performs multiple GET operations efficiently
func (ec *EnhancedCache) MGet(ctx context.Context, keys ...string) (map[string]*types.CacheEntry, error) {
	ec.stats.mu.Lock()
	ec.stats.BatchOperations++
	ec.stats.mu.Unlock()
	
	return ec.batchProcessor.MGet(ctx, keys...)
}

// MSet performs multiple SET operations efficiently
func (ec *EnhancedCache) MSet(ctx context.Context, pairs map[string][]byte, ttl time.Duration) error {
	ec.stats.mu.Lock()
	ec.stats.BatchOperations++
	ec.stats.mu.Unlock()
	
	if ttl == 0 {
		ttl = ec.config.DefaultTTL
	}
	return ec.batchProcessor.MSet(ctx, pairs, ttl)
}

// MDelete performs multiple DELETE operations efficiently
func (ec *EnhancedCache) MDelete(ctx context.Context, keys ...string) error {
	ec.stats.mu.Lock()
	ec.stats.BatchOperations++
	ec.stats.mu.Unlock()
	
	return ec.batchProcessor.MDelete(ctx, keys...)
}

// MSetObjects performs multiple object SET operations with serialization
func (ec *EnhancedCache) MSetObjects(ctx context.Context, objects map[string]interface{}, ttl time.Duration, format serialization.SerializationFormat) error {
	pairs := make(map[string][]byte)
	
	for key, obj := range objects {
		cacheValue, err := ec.serializer.SerializeValue(obj, format)
		if err != nil {
			return fmt.Errorf("serialization failed for key %s: %w", key, err)
		}
		
		data, err := json.Marshal(cacheValue)
		if err != nil {
			return fmt.Errorf("failed to marshal cache value for key %s: %w", key, err)
		}
		
		pairs[key] = data
	}
	
	return ec.MSet(ctx, pairs, ttl)
}

// MGetObjects performs multiple object GET operations with deserialization
func (ec *EnhancedCache) MGetObjects(ctx context.Context, keys []string, targets map[string]interface{}) error {
	entries, err := ec.MGet(ctx, keys...)
	if err != nil {
		return err
	}
	
	for key, entry := range entries {
		target, exists := targets[key]
		if !exists {
			continue
		}
		
		var cacheValue serialization.CacheValue
		if err := json.Unmarshal(entry.Value, &cacheValue); err != nil {
			return fmt.Errorf("failed to unmarshal cache value for key %s: %w", key, err)
		}
		
		if err := ec.serializer.DeserializeValue(&cacheValue, target); err != nil {
			return fmt.Errorf("deserialization failed for key %s: %w", key, err)
		}
	}
	
	return nil
}

// Connection pool operations

// GetConnection retrieves a connection from the pool
func (ec *EnhancedCache) GetConnection(address string) (*pool.PooledConnection, error) {
	if ec.connectionPool == nil {
		return nil, fmt.Errorf("connection pool not enabled")
	}
	
	factory := pool.NewTCPConnectionFactory(ec.config.PoolConfig.ConnectionTimeout)
	pool := ec.connectionPool.GetPool(address, factory, ec.config.PoolConfig)
	return pool.Get()
}

// Advanced cache patterns

// GetOrSet retrieves a value from cache or sets it if not found
func (ec *EnhancedCache) GetOrSet(ctx context.Context, key string, generator func() ([]byte, error), ttl time.Duration) (*types.CacheEntry, error) {
	// Try to get first
	if entry, err := ec.Get(ctx, key); err == nil {
		return entry, nil
	}
	
	// Generate value
	value, err := generator()
	if err != nil {
		return nil, fmt.Errorf("value generation failed: %w", err)
	}
	
	// Set in cache
	if err := ec.Set(ctx, key, value, ttl); err != nil {
		return nil, fmt.Errorf("failed to set generated value: %w", err)
	}
	
	// Return the entry
	return ec.Get(ctx, key)
}

// GetOrSetObject is like GetOrSet but with object serialization
func (ec *EnhancedCache) GetOrSetObject(ctx context.Context, key string, target interface{}, generator func() (interface{}, error), ttl time.Duration, format serialization.SerializationFormat) error {
	// Try to get first
	if err := ec.GetObject(ctx, key, target); err == nil {
		return nil
	}
	
	// Generate object
	obj, err := generator()
	if err != nil {
		return fmt.Errorf("object generation failed: %w", err)
	}
	
	// Set in cache
	if err := ec.SetObject(ctx, key, obj, ttl, format); err != nil {
		return fmt.Errorf("failed to set generated object: %w", err)
	}
	
	// Get the object back to populate target
	return ec.GetObject(ctx, key, target)
}

// Statistics and monitoring

// GetStats returns comprehensive cache statistics
func (ec *EnhancedCache) GetStats() *EnhancedCacheStats {
	ec.stats.mu.RLock()
	defer ec.stats.mu.RUnlock()
	
	stats := &EnhancedCacheStats{
		BatchOperations:     ec.stats.BatchOperations,
		PipelineExecutions:  ec.stats.PipelineExecutions,
		SerializationTime:   ec.stats.SerializationTime,
		DeserializationTime: ec.stats.DeserializationTime,
	}
	
	// Get basic cache stats
	if basicStats, ok := ec.storage.(interface{ GetStats() *types.CacheStats }); ok {
		stats.BasicStats = basicStats.GetStats()
	}
	
	// Get multi-level stats if available
	if mlCache, ok := ec.storage.(*multilevel.MultiLevelCache); ok {
		stats.MultiLevelStats = mlCache.GetStats()
	}
	
	return stats
}

// Close cleans up resources
func (ec *EnhancedCache) Close() error {
	var errors []error
	
	// Close connection pools
	if ec.connectionPool != nil {
		if err := ec.connectionPool.CloseAll(); err != nil {
			errors = append(errors, fmt.Errorf("connection pool close error: %w", err))
		}
	}
	
	// Close storage if it supports closing
	if closer, ok := ec.storage.(interface{ Close() error }); ok {
		if err := closer.Close(); err != nil {
			errors = append(errors, fmt.Errorf("storage close error: %w", err))
		}
	}
	
	if len(errors) > 0 {
		return fmt.Errorf("close errors: %v", errors)
	}
	
	return nil
}
