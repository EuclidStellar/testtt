package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
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
	advancedSerializer *serialization.AdvancedSerializationManager
	connectionPool  *pool.PoolManager
	
	// Configuration
	config          *EnhancedCacheConfig
	
	// Statistics
	stats           *EnhancedCacheStats
	mu              sync.RWMutex
	
	// Advanced features
	cacheWarmer     CacheWarmer
	ttlRefreshMgr   *TTLRefreshManager
	circuitBreaker  *CircuitBreaker
	
	// Lifecycle
	ctx             context.Context
	cancel          context.CancelFunc
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
	EnableCompression    bool
	EnableEncryption     bool
	EncryptionKey        []byte
	
	// Connection pooling
	EnableConnectionPool bool
	PoolConfig          *pool.PoolConfig
	
	// Multi-level cache
	EnableMultiLevel    bool
	TierConfigs         []multilevel.CacheTier
	
	// Advanced features
	EnableCacheWarming   bool
	EnableTTLRefresh     bool
	TTLRefreshThreshold  time.Duration
	EnableCircuitBreaker bool
	CircuitBreakerConfig *CircuitBreakerConfig
}

// CircuitBreakerConfig holds circuit breaker configuration
type CircuitBreakerConfig struct {
	MaxFailures  int
	ResetTimeout time.Duration
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
	CompressionRatio   float64
	EncryptionTime     time.Duration
	
	// Multi-level stats
	MultiLevelStats  *multilevel.MultiLevelStats
	
	// Connection pool stats
	PoolStats        pool.PoolStats
	
	// Advanced feature stats
	CacheWarmingTime time.Duration
	TTLRefreshes     int64
	CircuitBreakerTrips int64
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
		EnableCompression:    false,
		EnableEncryption:     false,
		EnableConnectionPool: true,
		PoolConfig:          pool.DefaultPoolConfig(),
		EnableMultiLevel:    false,
		TierConfigs:         []multilevel.CacheTier{},
		EnableCacheWarming:   false,
		EnableTTLRefresh:     false,
		TTLRefreshThreshold:  30 * time.Second,
		EnableCircuitBreaker: true,
		CircuitBreakerConfig: &CircuitBreakerConfig{
			MaxFailures:  5,
			ResetTimeout: 30 * time.Second,
		},
	}
}

// NewEnhancedCache creates a new enhanced cache instance
func NewEnhancedCache(config *EnhancedCacheConfig) (*EnhancedCache, error) {
	if config == nil {
		config = DefaultEnhancedCacheConfig()
	}
	
	// Create context for lifecycle management
	ctx, cancel := context.WithCancel(context.Background())
	
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
	
	// Create advanced serializer if needed
	var advancedSerializer *serialization.AdvancedSerializationManager
	if config.EnableCompression || config.EnableEncryption {
		advancedSerializer = serialization.NewAdvancedSerializationManager(
			config.DefaultSerialization,
			config.EncryptionKey,
		)
	}
	
	var connectionPool *pool.PoolManager
	if config.EnableConnectionPool {
		connectionPool = pool.NewPoolManager()
	}
	
	// Create circuit breaker
	var circuitBreaker *CircuitBreaker
	if config.EnableCircuitBreaker {
		circuitBreaker = NewCircuitBreaker(
			config.CircuitBreakerConfig.MaxFailures,
			config.CircuitBreakerConfig.ResetTimeout,
		)
	}
	
	cache := &EnhancedCache{
		storage:            storage,
		batchProcessor:     batchProcessor,
		serializer:         serializer,
		advancedSerializer: advancedSerializer,
		connectionPool:     connectionPool,
		config:             config,
		circuitBreaker:     circuitBreaker,
		ctx:                ctx,
		cancel:             cancel,
		stats: &EnhancedCacheStats{
			BasicStats: &types.CacheStats{
				MaxSize: config.MaxSize,
			},
		},
	}
	
	// Initialize TTL refresh manager if enabled
	if config.EnableTTLRefresh {
		cache.ttlRefreshMgr = NewTTLRefreshManager(cache, config.TTLRefreshThreshold)
		cache.ttlRefreshMgr.Start(ctx)
	}
	
	// Initialize cache warmer if enabled
	if config.EnableCacheWarming {
		cache.cacheWarmer = NewPreloadCacheWarmer()
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
func (ec *EnhancedCache) GetOrSetObject(ctx context.Context, key string, target interface{}, generator func() (interface{}, error), ttl time.Duration, format serialization.Serialization
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

// Enhanced statistics and monitoring
func (ec *EnhancedCache) GetDetailedStats() *EnhancedCacheStats {
	ec.stats.mu.RLock()
	defer ec.stats.mu.RUnlock()
	
	stats := &EnhancedCacheStats{
		BatchOperations:     ec.stats.BatchOperations,
		PipelineExecutions:  ec.stats.PipelineExecutions,
		SerializationTime:   ec.stats.SerializationTime,
		DeserializationTime: ec.stats.DeserializationTime,
		CompressionRatio:    ec.stats.CompressionRatio,
		EncryptionTime:      ec.stats.EncryptionTime,
		CacheWarmingTime:    ec.stats.CacheWarmingTime,
		TTLRefreshes:        ec.stats.TTLRefreshes,
		CircuitBreakerTrips: ec.stats.CircuitBreakerTrips,
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
	
	// Cancel context to stop background operations
	if ec.cancel != nil {
		ec.cancel()
	}
	
	// Stop TTL refresh manager
	if ec.ttlRefreshMgr != nil {
		ec.ttlRefreshMgr.Stop()
	}
	
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

// CacheWarmer interface for cache warming strategies
type CacheWarmer interface {
	WarmCache(ctx context.Context, cache *EnhancedCache) error
}

// PreloadCacheWarmer preloads specific keys
type PreloadCacheWarmer struct {
	entries map[string]CacheWarmEntry
}

type CacheWarmEntry struct {
	Value     interface{}
	TTL       time.Duration
	Format    serialization.SerializationFormat
}

func NewPreloadCacheWarmer() *PreloadCacheWarmer {
	return &PreloadCacheWarmer{
		entries: make(map[string]CacheWarmEntry),
	}
}

func (pcw *PreloadCacheWarmer) AddEntry(key string, value interface{}, ttl time.Duration, format serialization.SerializationFormat) {
	pcw.entries[key] = CacheWarmEntry{
		Value:  value,
		TTL:    ttl,
		Format: format,
	}
}

func (pcw *PreloadCacheWarmer) WarmCache(ctx context.Context, cache *EnhancedCache) error {
	for key, entry := range pcw.entries {
		if err := cache.SetObject(ctx, key, entry.Value, entry.TTL, entry.Format); err != nil {
			return fmt.Errorf("failed to warm cache for key %s: %w", key, err)
		}
	}
	return nil
}

// TTLRefreshManager handles automatic TTL refresh for frequently accessed items
type TTLRefreshManager struct {
	cache          *EnhancedCache
	refreshThreshold time.Duration
	refreshers     map[string]*TTLRefresher
	mu             sync.RWMutex
	stop           chan struct{}
	running        int32
}

type TTLRefresher struct {
	key            string
	refreshFunc    func(ctx context.Context, key string) (interface{}, time.Duration, error)
	lastAccess     time.Time
	accessCount    int64
	mu             sync.RWMutex
}

func NewTTLRefreshManager(cache *EnhancedCache, refreshThreshold time.Duration) *TTLRefreshManager {
	return &TTLRefreshManager{
		cache:            cache,
		refreshThreshold: refreshThreshold,
		refreshers:       make(map[string]*TTLRefresher),
		stop:             make(chan struct{}),
	}
}

func (trm *TTLRefreshManager) RegisterRefresher(key string, refreshFunc func(ctx context.Context, key string) (interface{}, time.Duration, error)) {
	trm.mu.Lock()
	defer trm.mu.Unlock()

	trm.refreshers[key] = &TTLRefresher{
		key:         key,
		refreshFunc: refreshFunc,
		lastAccess:  time.Now(),
	}
}

func (trm *TTLRefreshManager) Start(ctx context.Context) {
	if !atomic.CompareAndSwapInt32(&trm.running, 0, 1) {
		return // Already running
	}

	go trm.refreshLoop(ctx)
}

func (trm *TTLRefreshManager) Stop() {
	if atomic.CompareAndSwapInt32(&trm.running, 1, 0) {
		close(trm.stop)
	}
}

func (trm *TTLRefreshManager) refreshLoop(ctx context.Context) {
	ticker := time.NewTicker(trm.refreshThreshold / 2)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-trm.stop:
			return
		case <-ticker.C:
			trm.checkAndRefresh(ctx)
		}
	}
}

func (trm *TTLRefreshManager) checkAndRefresh(ctx context.Context) {
	trm.mu.RLock()
	refreshers := make([]*TTLRefresher, 0, len(trm.refreshers))
	for _, refresher := range trm.refreshers {
		refreshers = append(refreshers, refresher)
	}
	trm.mu.RUnlock()

	for _, refresher := range refreshers {
		go trm.refreshIfNeeded(ctx, refresher)
	}
}

func (trm *TTLRefreshManager) refreshIfNeeded(ctx context.Context, refresher *TTLRefresher) {
	refresher.mu.RLock()
	timeSinceAccess := time.Since(refresher.lastAccess)
	refresher.mu.RUnlock()

	if timeSinceAccess < trm.refreshThreshold {
		// Get current entry to check TTL
		entry, err := trm.cache.Get(ctx, refresher.key)
		if err != nil {
			return // Key doesn't exist or expired
		}

		timeUntilExpiry := time.Until(entry.ExpiresAt)
		if timeUntilExpiry < trm.refreshThreshold {
			// Refresh the entry
			value, ttl, err := refresher.refreshFunc(ctx, refresher.key)
			if err == nil {
				trm.cache.SetObject(ctx, refresher.key, value, ttl, serialization.JSON)
			}
		}
	}
}

func (trm *TTLRefreshManager) RecordAccess(key string) {
	trm.mu.RLock()
	refresher, exists := trm.refreshers[key]
	trm.mu.RUnlock()

	if exists {
		refresher.mu.Lock()
		refresher.lastAccess = time.Now()
		refresher.accessCount++
		refresher.mu.Unlock()
	}
}

// Circuit breaker for cache operations
type CircuitBreaker struct {
	maxFailures   int
	resetTimeout  time.Duration
	failures      int64
	lastFailTime  time.Time
	state         int32 // 0: closed, 1: open, 2: half-open
	mu            sync.RWMutex
}

func NewCircuitBreaker(maxFailures int, resetTimeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		maxFailures:  maxFailures,
		resetTimeout: resetTimeout,
	}
}

func (cb *CircuitBreaker) Execute(operation func() error) error {
	cb.mu.RLock()
	state := atomic.LoadInt32(&cb.state)
	cb.mu.RUnlock()

	if state == 1 { // Open state
		cb.mu.RLock()
		timeSinceLastFail := time.Since(cb.lastFailTime)
		cb.mu.RUnlock()

		if timeSinceLastFail > cb.resetTimeout {
			atomic.StoreInt32(&cb.state, 2) // Half-open
		} else {
			return fmt.Errorf("circuit breaker is open")
		}
	}

	err := operation()
	if err != nil {
		cb.recordFailure()
		return err
	}

	cb.recordSuccess()
	return nil
}

func (cb *CircuitBreaker) recordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	atomic.AddInt64(&cb.failures, 1)
	cb.lastFailTime = time.Now()

	if atomic.LoadInt64(&cb.failures) >= int64(cb.maxFailures) {
		atomic.StoreInt32(&cb.state, 1) // Open
	}
}

func (cb *CircuitBreaker) recordSuccess() {
	atomic.StoreInt64(&cb.failures, 0)
	atomic.StoreInt32(&cb.state, 0) // Closed
}
