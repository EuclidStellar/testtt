package cache

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yourusername/distributed-cache/pkg/serialization"
)

func TestEnhancedCache_BasicOperations(t *testing.T) {
	config := DefaultEnhancedCacheConfig()
	cache, err := NewEnhancedCache(config)
	require.NoError(t, err)
	defer cache.Close()

	ctx := context.Background()

	// Test Set and Get
	err = cache.Set(ctx, "key1", []byte("value1"), time.Minute)
	assert.NoError(t, err)

	entry, err := cache.Get(ctx, "key1")
	assert.NoError(t, err)
	assert.Equal(t, "key1", entry.Key)
	assert.Equal(t, []byte("value1"), entry.Value)

	// Test Exists
	exists, err := cache.Exists(ctx, "key1")
	assert.NoError(t, err)
	assert.True(t, exists)

	// Test Delete
	err = cache.Delete(ctx, "key1")
	assert.NoError(t, err)

	exists, err = cache.Exists(ctx, "key1")
	assert.NoError(t, err)
	assert.False(t, exists)
}

func TestEnhancedCache_ObjectSerialization(t *testing.T) {
	config := DefaultEnhancedCacheConfig()
	cache, err := NewEnhancedCache(config)
	require.NoError(t, err)
	defer cache.Close()

	ctx := context.Background()

	// Test struct serialization
	type TestStruct struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	original := TestStruct{Name: "test", Value: 42}

	// Test JSON serialization
	err = cache.SetObject(ctx, "struct_key", original, time.Minute, serialization.JSON)
	assert.NoError(t, err)

	var retrieved TestStruct
	err = cache.GetObject(ctx, "struct_key", &retrieved)
	assert.NoError(t, err)
	assert.Equal(t, original, retrieved)

	// Test MessagePack serialization
	err = cache.SetObject(ctx, "msgpack_key", original, time.Minute, serialization.MSGPACK)
	assert.NoError(t, err)

	var msgpackRetrieved TestStruct
	err = cache.GetObject(ctx, "msgpack_key", &msgpackRetrieved)
	assert.NoError(t, err)
	assert.Equal(t, original, msgpackRetrieved)
}

func TestEnhancedCache_AdvancedOperations(t *testing.T) {
	config := DefaultEnhancedCacheConfig()
	config.EnableCompression = true
	config.EnableEncryption = true
	config.EncryptionKey = []byte("my-secret-key-32-bytes-long!!!!") // 32 bytes for AES-256
	
	cache, err := NewEnhancedCache(config)
	require.NoError(t, err)
	defer cache.Close()

	ctx := context.Background()

	type TestData struct {
		Message string `json:"message"`
		Numbers []int  `json:"numbers"`
	}

	original := TestData{
		Message: "This is a test message that should be compressed and encrypted",
		Numbers: []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10},
	}

	// Test advanced serialization with compression and encryption
	err = cache.SetObjectAdvanced(ctx, "advanced_key", original, time.Minute, serialization.JSON, true, true)
	assert.NoError(t, err)

	var retrieved TestData
	err = cache.GetObjectAdvanced(ctx, "advanced_key", &retrieved)
	assert.NoError(t, err)
	assert.Equal(t, original, retrieved)
}

func TestEnhancedCache_BatchOperations(t *testing.T) {
	config := DefaultEnhancedCacheConfig()
	cache, err := NewEnhancedCache(config)
	require.NoError(t, err)
	defer cache.Close()

	ctx := context.Background()

	// Test MSet
	pairs := map[string][]byte{
		"batch_key1": []byte("value1"),
		"batch_key2": []byte("value2"),
		"batch_key3": []byte("value3"),
	}

	err = cache.MSet(ctx, pairs, time.Minute)
	assert.NoError(t, err)

	// Test MGet
	keys := []string{"batch_key1", "batch_key2", "batch_key3"}
	results, err := cache.MGet(ctx, keys...)
	assert.NoError(t, err)
	assert.Len(t, results, 3)

	for key, expectedValue := range pairs {
		entry, exists := results[key]
		assert.True(t, exists)
		assert.Equal(t, expectedValue, entry.Value)
	}

	// Test MDelete
	err = cache.MDelete(ctx, keys...)
	assert.NoError(t, err)

	// Verify deletion
	for _, key := range keys {
		exists, err := cache.Exists(ctx, key)
		assert.NoError(t, err)
		assert.False(t, exists)
	}
}

func TestEnhancedCache_Pipeline(t *testing.T) {
	config := DefaultEnhancedCacheConfig()
	cache, err := NewEnhancedCache(config)
	require.NoError(t, err)
	defer cache.Close()

	ctx := context.Background()

	// Create a pipeline
	pipeline := cache.Pipeline()

	// Add operations to pipeline
	pipeline.Set("pipe_key1", []byte("value1"), time.Minute)
	pipeline.Set("pipe_key2", []byte("value2"), time.Minute)
	pipeline.Get("pipe_key1")
	pipeline.Get("pipe_key2")

	// Execute pipeline
	results, err := cache.ExecutePipeline(ctx, pipeline)
	assert.NoError(t, err)
	assert.Len(t, results, 4)

	// Check results
	assert.NoError(t, results[0].Error) // SET operation
	assert.NoError(t, results[1].Error) // SET operation
	assert.NoError(t, results[2].Error) // GET operation
	assert.NoError(t, results[3].Error) // GET operation

	assert.Equal(t, []byte("value1"), results[2].Value.Value)
	assert.Equal(t, []byte("value2"), results[3].Value.Value)
}

func TestEnhancedCache_TTLExpiration(t *testing.T) {
	config := DefaultEnhancedCacheConfig()
	cache, err := NewEnhancedCache(config)
	require.NoError(t, err)
	defer cache.Close()

	ctx := context.Background()

	// Set with short TTL
	err = cache.Set(ctx, "ttl_key", []byte("ttl_value"), 100*time.Millisecond)
	assert.NoError(t, err)

	// Should exist immediately
	exists, err := cache.Exists(ctx, "ttl_key")
	assert.NoError(t, err)
	assert.True(t, exists)

	// Wait for expiration
	time.Sleep(150 * time.Millisecond)

	// Should not exist after expiration
	exists, err = cache.Exists(ctx, "ttl_key")
	assert.NoError(t, err)
	assert.False(t, exists)
}

func TestEnhancedCache_CacheWarming(t *testing.T) {
	config := DefaultEnhancedCacheConfig()
	config.EnableCacheWarming = true
	
	cache, err := NewEnhancedCache(config)
	require.NoError(t, err)
	defer cache.Close()

	ctx := context.Background()

	// Add warmup entries
	err = cache.AddWarmupEntry("warm_key1", "warm_value1", time.Hour, serialization.JSON)
	assert.NoError(t, err)

	err = cache.AddWarmupEntry("warm_key2", map[string]int{"count": 42}, time.Hour, serialization.JSON)
	assert.NoError(t, err)

	// Warm the cache
	err = cache.WarmCache(ctx)
	assert.NoError(t, err)

	// Verify warmed entries exist
	var stringValue string
	err = cache.GetObject(ctx, "warm_key1", &stringValue)
	assert.NoError(t, err)
	assert.Equal(t, "warm_value1", stringValue)

	var mapValue map[string]int
	err = cache.GetObject(ctx, "warm_key2", &mapValue)
	assert.NoError(t, err)
	assert.Equal(t, map[string]int{"count": 42}, mapValue)
}

func TestEnhancedCache_GetOrSet(t *testing.T) {
	config := DefaultEnhancedCacheConfig()
	cache, err := NewEnhancedCache(config)
	require.NoError(t, err)
	defer cache.Close()

	ctx := context.Background()

	// Test GetOrSet with generator function
	callCount := 0
	generator := func() ([]byte, error) {
		callCount++
		return []byte("generated_value"), nil
	}

	// First call should generate
	entry, err := cache.GetOrSet(ctx, "getorset_key", generator, time.Minute)
	assert.NoError(t, err)
	assert.Equal(t, []byte("generated_value"), entry.Value)
	assert.Equal(t, 1, callCount)

	// Second call should use cached value
	entry, err = cache.GetOrSet(ctx, "getorset_key", generator, time.Minute)
	assert.NoError(t, err)
	assert.Equal(t, []byte("generated_value"), entry.Value)
	assert.Equal(t, 1, callCount) // Should not increment
}

func TestEnhancedCache_Statistics(t *testing.T) {
	config := DefaultEnhancedCacheConfig()
	cache, err := NewEnhancedCache(config)
	require.NoError(t, err)
	defer cache.Close()

	ctx := context.Background()

	// Perform some operations
	cache.Set(ctx, "stats_key1", []byte("value1"), time.Minute)
	cache.Set(ctx, "stats_key2", []byte("value2"), time.Minute)
	cache.Get(ctx, "stats_key1")
	cache.Get(ctx, "stats_key2")
	cache.Get(ctx, "nonexistent_key") // This should be a miss

	// Get statistics
	stats := cache.GetDetailedStats()
	assert.NotNil(t, stats)
	assert.NotNil(t, stats.BasicStats)
	
	// Should have some hits and misses
	assert.True(t, stats.BasicStats.Hits > 0)
	assert.True(t, stats.BasicStats.Misses > 0)
}

// Benchmark tests
func BenchmarkEnhancedCache_Set(b *testing.B) {
	config := DefaultEnhancedCacheConfig()
	cache, _ := NewEnhancedCache(config)
	defer cache.Close()

	ctx := context.Background()
	value := []byte("benchmark_value")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Set(ctx, fmt.Sprintf("bench_key_%d", i), value, time.Minute)
	}
}

func BenchmarkEnhancedCache_Get(b *testing.B) {
	config := DefaultEnhancedCacheConfig()
	cache, _ := NewEnhancedCache(config)
	defer cache.Close()

	ctx := context.Background()
	value := []byte("benchmark_value")

	// Pre-populate cache
	for i := 0; i < 1000; i++ {
		cache.Set(ctx, fmt.Sprintf("bench_key_%d", i), value, time.Minute)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Get(ctx, fmt.Sprintf("bench_key_%d", i%1000))
	}
}

func BenchmarkEnhancedCache_SetObjectAdvanced(b *testing.B) {
	config := DefaultEnhancedCacheConfig()
	config.EnableCompression = true
	config.EnableEncryption = true
	config.EncryptionKey = []byte("my-secret-key-32-bytes-long!!!!")

	cache, _ := NewEnhancedCache(config)
	defer cache.Close()

	ctx := context.Background()
	
	type BenchData struct {
		Message string   `json:"message"`
		Numbers []int    `json:"numbers"`
		Data    []string `json:"data"`
	}

	testData := BenchData{
		Message: "This is a benchmark test message with some content to compress",
		Numbers: make([]int, 100),
		Data:    make([]string, 50),
	}

	for i := range testData.Numbers {
		testData.Numbers[i] = i
	}
	for i := range testData.Data {
		testData.Data[i] = fmt.Sprintf("data_item_%d", i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.SetObjectAdvanced(ctx, fmt.Sprintf("bench_obj_%d", i), testData, time.Minute, serialization.JSON, true, true)
	}
}
