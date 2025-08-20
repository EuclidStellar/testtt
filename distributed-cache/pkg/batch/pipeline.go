package batch

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/yourusername/distributed-cache/pkg/types"
)

// BatchOperation represents a single operation in a batch
type BatchOperation struct {
	Type     OperationType
	Key      string
	Value    []byte
	TTL      time.Duration
	Result   chan BatchResult
}

// OperationType defines the type of batch operation
type OperationType int

const (
	GET OperationType = iota
	SET
	DELETE
	EXISTS
)

// BatchResult holds the result of a batch operation
type BatchResult struct {
	Value *types.CacheEntry
	Error error
	Found bool
}

// Pipeline represents a batch of operations
type Pipeline struct {
	operations []BatchOperation
	executed   bool
	mu         sync.RWMutex
}

// NewPipeline creates a new pipeline for batch operations
func NewPipeline() *Pipeline {
	return &Pipeline{
		operations: make([]BatchOperation, 0),
		executed:   false,
	}
}

// Get adds a GET operation to the pipeline
func (p *Pipeline) Get(key string) *Pipeline {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	if p.executed {
		panic("pipeline already executed")
	}
	
	op := BatchOperation{
		Type:   GET,
		Key:    key,
		Result: make(chan BatchResult, 1),
	}
	
	p.operations = append(p.operations, op)
	return p
}

// Set adds a SET operation to the pipeline
func (p *Pipeline) Set(key string, value []byte, ttl time.Duration) *Pipeline {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	if p.executed {
		panic("pipeline already executed")
	}
	
	op := BatchOperation{
		Type:   SET,
		Key:    key,
		Value:  value,
		TTL:    ttl,
		Result: make(chan BatchResult, 1),
	}
	
	p.operations = append(p.operations, op)
	return p
}

// Delete adds a DELETE operation to the pipeline
func (p *Pipeline) Delete(key string) *Pipeline {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	if p.executed {
		panic("pipeline already executed")
	}
	
	op := BatchOperation{
		Type:   DELETE,
		Key:    key,
		Result: make(chan BatchResult, 1),
	}
	
	p.operations = append(p.operations, op)
	return p
}

// Exists adds an EXISTS operation to the pipeline
func (p *Pipeline) Exists(key string) *Pipeline {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	if p.executed {
		panic("pipeline already executed")
	}
	
	op := BatchOperation{
		Type:   EXISTS,
		Key:    key,
		Result: make(chan BatchResult, 1),
	}
	
	p.operations = append(p.operations, op)
	return p
}

// MGet adds multiple GET operations to the pipeline
func (p *Pipeline) MGet(keys ...string) *Pipeline {
	for _, key := range keys {
		p.Get(key)
	}
	return p
}

// MSet adds multiple SET operations to the pipeline
func (p *Pipeline) MSet(pairs map[string][]byte, ttl time.Duration) *Pipeline {
	for key, value := range pairs {
		p.Set(key, value, ttl)
	}
	return p
}

// Execute runs all operations in the pipeline
func (p *Pipeline) Execute(ctx context.Context, cache types.Cache) ([]BatchResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	if p.executed {
		return nil, ErrPipelineAlreadyExecuted
	}
	
	p.executed = true
	results := make([]BatchResult, len(p.operations))
	
	// Execute operations concurrently
	var wg sync.WaitGroup
	for i, op := range p.operations {
		wg.Add(1)
		go func(idx int, operation BatchOperation) {
			defer wg.Done()
			
			var result BatchResult
			
			switch operation.Type {
			case GET:
				entry, err := cache.Get(ctx, operation.Key)
				result = BatchResult{
					Value: entry,
					Error: err,
					Found: err == nil,
				}
			case SET:
				err := cache.Set(ctx, operation.Key, operation.Value, operation.TTL)
				result = BatchResult{
					Error: err,
					Found: err == nil,
				}
			case DELETE:
				err := cache.Delete(ctx, operation.Key)
				result = BatchResult{
					Error: err,
					Found: err == nil,
				}
			case EXISTS:
				exists, err := cache.Exists(ctx, operation.Key)
				result = BatchResult{
					Error: err,
					Found: exists,
				}
			}
			
			results[idx] = result
			operation.Result <- result
		}(i, op)
	}
	
	wg.Wait()
	return results, nil
}

// GetResults returns results for each operation in order
func (p *Pipeline) GetResults() []BatchResult {
	p.mu.RLock()
	defer p.mu.RUnlock()
	
	if !p.executed {
		return nil
	}
	
	results := make([]BatchResult, len(p.operations))
	for i, op := range p.operations {
		select {
		case result := <-op.Result:
			results[i] = result
		default:
			results[i] = BatchResult{Error: ErrResultNotReady}
		}
	}
	
	return results
}

// BatchCache extends the basic cache interface with batch operations
type BatchCache interface {
	types.Cache
	Pipeline() *Pipeline
	ExecutePipeline(ctx context.Context, pipeline *Pipeline) ([]BatchResult, error)
	MGet(ctx context.Context, keys ...string) (map[string]*types.CacheEntry, error)
	MSet(ctx context.Context, pairs map[string][]byte, ttl time.Duration) error
	MDelete(ctx context.Context, keys ...string) error
}

// BatchProcessor handles batch operations efficiently
type BatchProcessor struct {
	cache       types.Cache
	maxBatchSize int
	timeout     time.Duration
}

// NewBatchProcessor creates a new batch processor
func NewBatchProcessor(cache types.Cache, maxBatchSize int, timeout time.Duration) *BatchProcessor {
	return &BatchProcessor{
		cache:       cache,
		maxBatchSize: maxBatchSize,
		timeout:     timeout,
	}
}

// MGet performs multiple GET operations efficiently
func (bp *BatchProcessor) MGet(ctx context.Context, keys ...string) (map[string]*types.CacheEntry, error) {
	results := make(map[string]*types.CacheEntry)
	resultsMu := sync.Mutex{}
	
	// Process in batches
	for i := 0; i < len(keys); i += bp.maxBatchSize {
		end := i + bp.maxBatchSize
		if end > len(keys) {
			end = len(keys)
		}
		
		batch := keys[i:end]
		var wg sync.WaitGroup
		
		for _, key := range batch {
			wg.Add(1)
			go func(k string) {
				defer wg.Done()
				
				if entry, err := bp.cache.Get(ctx, k); err == nil {
					resultsMu.Lock()
					results[k] = entry
					resultsMu.Unlock()
				}
			}(key)
		}
		
		wg.Wait()
	}
	
	return results, nil
}

// MSet performs multiple SET operations efficiently
func (bp *BatchProcessor) MSet(ctx context.Context, pairs map[string][]byte, ttl time.Duration) error {
	keys := make([]string, 0, len(pairs))
	for key := range pairs {
		keys = append(keys, key)
	}
	
	// Process in batches
	for i := 0; i < len(keys); i += bp.maxBatchSize {
		end := i + bp.maxBatchSize
		if end > len(keys) {
			end = len(keys)
		}
		
		batch := keys[i:end]
		var wg sync.WaitGroup
		errChan := make(chan error, len(batch))
		
		for _, key := range batch {
			wg.Add(1)
			go func(k string) {
				defer wg.Done()
				err := bp.cache.Set(ctx, k, pairs[k], ttl)
				if err != nil {
					errChan <- err
				}
			}(key)
		}
		
		wg.Wait()
		close(errChan)
		
		// Check for errors
		for err := range errChan {
			if err != nil {
				return err
			}
		}
	}
	
	return nil
}

// MDelete performs multiple DELETE operations efficiently
func (bp *BatchProcessor) MDelete(ctx context.Context, keys ...string) error {
	// Process in batches
	for i := 0; i < len(keys); i += bp.maxBatchSize {
		end := i + bp.maxBatchSize
		if end > len(keys) {
			end = len(keys)
		}
		
		batch := keys[i:end]
		var wg sync.WaitGroup
		errChan := make(chan error, len(batch))
		
		for _, key := range batch {
			wg.Add(1)
			go func(k string) {
				defer wg.Done()
				err := bp.cache.Delete(ctx, k)
				if err != nil {
					errChan <- err
				}
			}(key)
		}
		
		wg.Wait()
		close(errChan)
		
		// Check for errors
		for err := range errChan {
			if err != nil {
				return err
			}
		}
	}
	
	return nil
}

// Common errors
var (
	ErrPipelineAlreadyExecuted = fmt.Errorf("pipeline already executed")
	ErrResultNotReady         = fmt.Errorf("result not ready")
)
