package multilevel

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/yourusername/distributed-cache/pkg/types"
)

// CacheLevel defines different cache levels
type CacheLevel int

const (
	L1Cache CacheLevel = iota // In-memory (fastest)
	L2Cache                   // Local persistent cache
	L3Cache                   // Distributed cache
)

// CacheTier represents a single cache tier
type CacheTier struct {
	Cache    types.Cache
	Level    CacheLevel
	Priority int
	TTL      time.Duration
}

// PromotionPolicy defines when to promote data between tiers
type PromotionPolicy interface {
	ShouldPromote(key string, level CacheLevel, accessCount int64, lastAccess time.Time) bool
	ShouldDemote(key string, level CacheLevel, accessCount int64, lastAccess time.Time) bool
}

// DefaultPromotionPolicy implements a simple promotion policy
type DefaultPromotionPolicy struct {
	L1PromotionThreshold int64         // Access count threshold for L1 promotion
	L2PromotionThreshold int64         // Access count threshold for L2 promotion
	DemotionIdleTime     time.Duration // Time after which to consider demotion
}

func (dpp *DefaultPromotionPolicy) ShouldPromote(key string, level CacheLevel, accessCount int64, lastAccess time.Time) bool {
	switch level {
	case L2Cache:
		return accessCount >= dpp.L1PromotionThreshold
	case L3Cache:
		return accessCount >= dpp.L2PromotionThreshold
	}
	return false
}

func (dpp *DefaultPromotionPolicy) ShouldDemote(key string, level CacheLevel, accessCount int64, lastAccess time.Time) bool {
	idleTime := time.Since(lastAccess)
	return idleTime > dpp.DemotionIdleTime
}

// MultiLevelCache implements a hierarchical cache system
type MultiLevelCache struct {
	mu              sync.RWMutex
	tiers           []CacheTier
	promotionPolicy PromotionPolicy
	stats           *MultiLevelStats
	
	// Async operations
	promotionCh   chan promotionTask
	demotionCh    chan demotionTask
	cleanupTicker *time.Ticker
	stopCh        chan struct{}
}

type promotionTask struct {
	key     string
	value   *types.CacheEntry
	fromLevel CacheLevel
	toLevel   CacheLevel
}

type demotionTask struct {
	key     string
	fromLevel CacheLevel
	toLevel   CacheLevel
}

// MultiLevelStats tracks statistics for multi-level cache
type MultiLevelStats struct {
	mu sync.RWMutex
	
	L1Hits   int64
	L1Misses int64
	L2Hits   int64
	L2Misses int64
	L3Hits   int64
	L3Misses int64
	
	Promotions int64
	Demotions  int64
	
	TotalRequests int64
	L1HitRatio    float64
	L2HitRatio    float64
	L3HitRatio    float64
	OverallHitRatio float64
}

// NewMultiLevelCache creates a new multi-level cache
func NewMultiLevelCache(tiers []CacheTier, policy PromotionPolicy) *MultiLevelCache {
	if policy == nil {
		policy = &DefaultPromotionPolicy{
			L1PromotionThreshold: 5,
			L2PromotionThreshold: 3,
			DemotionIdleTime:     30 * time.Minute,
		}
	}
	
	mlc := &MultiLevelCache{
		tiers:           tiers,
		promotionPolicy: policy,
		stats:           &MultiLevelStats{},
		promotionCh:     make(chan promotionTask, 1000),
		demotionCh:      make(chan demotionTask, 1000),
		cleanupTicker:   time.NewTicker(5 * time.Minute),
		stopCh:          make(chan struct{}),
	}
	
	// Start background workers
	go mlc.promotionWorker()
	go mlc.demotionWorker()
	go mlc.cleanupWorker()
	
	return mlc
}

// Get retrieves a value from the cache hierarchy
func (mlc *MultiLevelCache) Get(ctx context.Context, key string) (*types.CacheEntry, error) {
	mlc.stats.mu.Lock()
	mlc.stats.TotalRequests++
	mlc.stats.mu.Unlock()
	
	// Try each tier in order
	for i, tier := range mlc.tiers {
		entry, err := tier.Cache.Get(ctx, key)
		if err == nil {
			// Cache hit - update stats and consider promotion
			mlc.updateHitStats(tier.Level)
			
			// Backfill higher priority tiers
			go mlc.backfillTiers(key, entry, i)
			
			// Check for promotion
			if mlc.promotionPolicy.ShouldPromote(key, tier.Level, entry.AccessCount, entry.AccessedAt) {
				mlc.schedulePromotion(key, entry, tier.Level)
			}
			
			return entry, nil
		}
		
		// Cache miss
		mlc.updateMissStats(tier.Level)
	}
	
	return nil, fmt.Errorf("key not found in any cache tier: %s", key)
}

// Set stores a value in the cache hierarchy
func (mlc *MultiLevelCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	// Store in all tiers with appropriate TTLs
	var errors []error
	
	for _, tier := range mlc.tiers {
		tierTTL := ttl
		if tier.TTL > 0 && tier.TTL < ttl {
			tierTTL = tier.TTL
		}
		
		if err := tier.Cache.Set(ctx, key, value, tierTTL); err != nil {
			errors = append(errors, fmt.Errorf("tier %d error: %w", tier.Level, err))
		}
	}
	
	if len(errors) > 0 {
		return fmt.Errorf("some tiers failed: %v", errors)
	}
	
	return nil
}

// Delete removes a key from all cache tiers
func (mlc *MultiLevelCache) Delete(ctx context.Context, key string) error {
	var errors []error
	
	for _, tier := range mlc.tiers {
		if err := tier.Cache.Delete(ctx, key); err != nil {
			errors = append(errors, fmt.Errorf("tier %d error: %w", tier.Level, err))
		}
	}
	
	if len(errors) > 0 {
		return fmt.Errorf("some tiers failed: %v", errors)
	}
	
	return nil
}

// Exists checks if a key exists in any cache tier
func (mlc *MultiLevelCache) Exists(ctx context.Context, key string) (bool, error) {
	for _, tier := range mlc.tiers {
		exists, err := tier.Cache.Exists(ctx, key)
		if err != nil {
			continue
		}
		if exists {
			return true, nil
		}
	}
	return false, nil
}

// Size returns the total size across all tiers
func (mlc *MultiLevelCache) Size() int64 {
	var total int64
	for _, tier := range mlc.tiers {
		total += tier.Cache.Size()
	}
	return total
}

// Clear clears all cache tiers
func (mlc *MultiLevelCache) Clear() error {
	var errors []error
	
	for _, tier := range mlc.tiers {
		if err := tier.Cache.Clear(); err != nil {
			errors = append(errors, fmt.Errorf("tier %d error: %w", tier.Level, err))
		}
	}
	
	if len(errors) > 0 {
		return fmt.Errorf("some tiers failed: %v", errors)
	}
	
	return nil
}

// backfillTiers copies data to higher priority tiers
func (mlc *MultiLevelCache) backfillTiers(key string, entry *types.CacheEntry, hitTierIndex int) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	// Backfill to higher priority tiers (lower indices)
	for i := 0; i < hitTierIndex; i++ {
		tier := mlc.tiers[i]
		tierTTL := entry.ExpiresAt.Sub(time.Now())
		if tier.TTL > 0 && tier.TTL < tierTTL {
			tierTTL = tier.TTL
		}
		
		// Don't block on backfill errors
		go func(t CacheTier, ttl time.Duration) {
			t.Cache.Set(ctx, key, entry.Value, ttl)
		}(tier, tierTTL)
	}
}

func (mlc *MultiLevelCache) updateHitStats(level CacheLevel) {
	mlc.stats.mu.Lock()
	defer mlc.stats.mu.Unlock()
	
	switch level {
	case L1Cache:
		mlc.stats.L1Hits++
	case L2Cache:
		mlc.stats.L2Hits++
	case L3Cache:
		mlc.stats.L3Hits++
	}
	
	mlc.updateHitRatios()
}

func (mlc *MultiLevelCache) updateMissStats(level CacheLevel) {
	mlc.stats.mu.Lock()
	defer mlc.stats.mu.Unlock()
	
	switch level {
	case L1Cache:
		mlc.stats.L1Misses++
	case L2Cache:
		mlc.stats.L2Misses++
	case L3Cache:
		mlc.stats.L3Misses++
	}
	
	mlc.updateHitRatios()
}

func (mlc *MultiLevelCache) updateHitRatios() {
	// Calculate hit ratios
	l1Total := mlc.stats.L1Hits + mlc.stats.L1Misses
	if l1Total > 0 {
		mlc.stats.L1HitRatio = float64(mlc.stats.L1Hits) / float64(l1Total)
	}
	
	l2Total := mlc.stats.L2Hits + mlc.stats.L2Misses
	if l2Total > 0 {
		mlc.stats.L2HitRatio = float64(mlc.stats.L2Hits) / float64(l2Total)
	}
	
	l3Total := mlc.stats.L3Hits + mlc.stats.L3Misses
	if l3Total > 0 {
		mlc.stats.L3HitRatio = float64(mlc.stats.L3Hits) / float64(l3Total)
	}
	
	totalHits := mlc.stats.L1Hits + mlc.stats.L2Hits + mlc.stats.L3Hits
	if mlc.stats.TotalRequests > 0 {
		mlc.stats.OverallHitRatio = float64(totalHits) / float64(mlc.stats.TotalRequests)
	}
}

func (mlc *MultiLevelCache) schedulePromotion(key string, entry *types.CacheEntry, fromLevel CacheLevel) {
	var toLevel CacheLevel
	
	switch fromLevel {
	case L3Cache:
		toLevel = L2Cache
	case L2Cache:
		toLevel = L1Cache
	default:
		return // Already at highest level
	}
	
	task := promotionTask{
		key:       key,
		value:     entry,
		fromLevel: fromLevel,
		toLevel:   toLevel,
	}
	
	select {
	case mlc.promotionCh <- task:
		// Scheduled successfully
	default:
		// Channel full, drop promotion
	}
}

func (mlc *MultiLevelCache) promotionWorker() {
	for {
		select {
		case task := <-mlc.promotionCh:
			mlc.executePromotion(task)
		case <-mlc.stopCh:
			return
		}
	}
}

func (mlc *MultiLevelCache) executePromotion(task promotionTask) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	// Find target tier
	var targetTier *CacheTier
	for i := range mlc.tiers {
		if mlc.tiers[i].Level == task.toLevel {
			targetTier = &mlc.tiers[i]
			break
		}
	}
	
	if targetTier == nil {
		return
	}
	
	// Calculate TTL
	ttl := task.value.ExpiresAt.Sub(time.Now())
	if targetTier.TTL > 0 && targetTier.TTL < ttl {
		ttl = targetTier.TTL
	}
	
	// Promote
	if err := targetTier.Cache.Set(ctx, task.key, task.value.Value, ttl); err == nil {
		mlc.stats.mu.Lock()
		mlc.stats.Promotions++
		mlc.stats.mu.Unlock()
	}
}

func (mlc *MultiLevelCache) demotionWorker() {
	for {
		select {
		case task := <-mlc.demotionCh:
			mlc.executeDemotion(task)
		case <-mlc.stopCh:
			return
		}
	}
}

func (mlc *MultiLevelCache) executeDemotion(task demotionTask) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	// Find source tier
	var sourceTier *CacheTier
	for i := range mlc.tiers {
		if mlc.tiers[i].Level == task.fromLevel {
			sourceTier = &mlc.tiers[i]
			break
		}
	}
	
	if sourceTier == nil {
		return
	}
	
	// Remove from source tier
	if err := sourceTier.Cache.Delete(ctx, task.key); err == nil {
		mlc.stats.mu.Lock()
		mlc.stats.Demotions++
		mlc.stats.mu.Unlock()
	}
}

func (mlc *MultiLevelCache) cleanupWorker() {
	for {
		select {
		case <-mlc.cleanupTicker.C:
			mlc.performCleanup()
		case <-mlc.stopCh:
			return
		}
	}
}

func (mlc *MultiLevelCache) performCleanup() {
	// This is a placeholder for cleanup logic
	// In a real implementation, you might:
	// 1. Check for items to demote based on access patterns
	// 2. Clean up expired entries
	// 3. Rebalance between tiers
	// 4. Update promotion/demotion policies based on stats
}

// GetStats returns current multi-level cache statistics
func (mlc *MultiLevelCache) GetStats() *MultiLevelStats {
	mlc.stats.mu.RLock()
	defer mlc.stats.mu.RUnlock()
	
	// Return a copy
	statsCopy := *mlc.stats
	return &statsCopy
}

// Close shuts down the multi-level cache
func (mlc *MultiLevelCache) Close() error {
	close(mlc.stopCh)
	mlc.cleanupTicker.Stop()
	
	var errors []error
	for _, tier := range mlc.tiers {
		if cleaner, ok := tier.Cache.(interface{ Close() error }); ok {
			if err := cleaner.Close(); err != nil {
				errors = append(errors, err)
			}
		}
	}
	
	if len(errors) > 0 {
		return fmt.Errorf("some tiers failed to close: %v", errors)
	}
	
	return nil
}

// HotDataPromotion implements a more sophisticated promotion policy
type HotDataPromotion struct {
	mu                   sync.RWMutex
	keyAccessCount       map[string]int64
	keyLastAccess        map[string]time.Time
	l1PromotionThreshold int64
	l2PromotionThreshold int64
	hotDataWindow        time.Duration
	demotionIdleTime     time.Duration
}

func NewHotDataPromotion(l1Threshold, l2Threshold int64, hotWindow, demotionIdle time.Duration) *HotDataPromotion {
	return &HotDataPromotion{
		keyAccessCount:       make(map[string]int64),
		keyLastAccess:        make(map[string]time.Time),
		l1PromotionThreshold: l1Threshold,
		l2PromotionThreshold: l2Threshold,
		hotDataWindow:        hotWindow,
		demotionIdleTime:     demotionIdle,
	}
}

func (hdp *HotDataPromotion) ShouldPromote(key string, level CacheLevel, accessCount int64, lastAccess time.Time) bool {
	hdp.mu.Lock()
	defer hdp.mu.Unlock()
	
	hdp.keyAccessCount[key] = accessCount
	hdp.keyLastAccess[key] = lastAccess
	
	// Check if data is "hot" based on recent access frequency
	recentAccesses := hdp.getRecentAccessCount(key)
	
	switch level {
	case L2Cache:
		return recentAccesses >= hdp.l1PromotionThreshold
	case L3Cache:
		return recentAccesses >= hdp.l2PromotionThreshold
	}
	
	return false
}

func (hdp *HotDataPromotion) ShouldDemote(key string, level CacheLevel, accessCount int64, lastAccess time.Time) bool {
	hdp.mu.RLock()
	defer hdp.mu.RUnlock()
	
	idleTime := time.Since(lastAccess)
	return idleTime > hdp.demotionIdleTime
}

func (hdp *HotDataPromotion) getRecentAccessCount(key string) int64 {
	// This is a simplified implementation
	// In practice, you might track access counts in time windows
	return hdp.keyAccessCount[key]
}
