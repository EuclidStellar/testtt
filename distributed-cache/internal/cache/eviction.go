package cache

import (
	"container/list"
	"sync"

	"github.com/yourusername/distributed-cache/pkg/types"
)

// EvictionStrategy defines the interface for eviction policies
type EvictionStrategy interface {
	OnAccess(key string)
	OnInsert(key string, size int64)
	OnDelete(key string)
	EvictOldest() string
	Clear()
	Size() int
}

// LRUEviction implements Least Recently Used eviction
type LRUEviction struct {
	mu       sync.RWMutex
	capacity int
	items    map[string]*list.Element
	order    *list.List
}

type lruItem struct {
	key  string
	size int64
}

// NewLRUEviction creates a new LRU eviction policy
func NewLRUEviction(capacity int) *LRUEviction {
	return &LRUEviction{
		capacity: capacity,
		items:    make(map[string]*list.Element),
		order:    list.New(),
	}
}

func (lru *LRUEviction) OnAccess(key string) {
	lru.mu.Lock()
	defer lru.mu.Unlock()
	
	if elem, exists := lru.items[key]; exists {
		lru.order.MoveToFront(elem)
	}
}

func (lru *LRUEviction) OnInsert(key string, size int64) {
	lru.mu.Lock()
	defer lru.mu.Unlock()
	
	if elem, exists := lru.items[key]; exists {
		lru.order.MoveToFront(elem)
		elem.Value.(*lruItem).size = size
		return
	}
	
	item := &lruItem{key: key, size: size}
	elem := lru.order.PushFront(item)
	lru.items[key] = elem
}

func (lru *LRUEviction) OnDelete(key string) {
	lru.mu.Lock()
	defer lru.mu.Unlock()
	
	if elem, exists := lru.items[key]; exists {
		lru.order.Remove(elem)
		delete(lru.items, key)
	}
}

func (lru *LRUEviction) EvictOldest() string {
	lru.mu.Lock()
	defer lru.mu.Unlock()
	
	if lru.order.Len() == 0 {
		return ""
	}
	
	elem := lru.order.Back()
	if elem != nil {
		item := elem.Value.(*lruItem)
		lru.order.Remove(elem)
		delete(lru.items, item.key)
		return item.key
	}
	
	return ""
}

func (lru *LRUEviction) Clear() {
	lru.mu.Lock()
	defer lru.mu.Unlock()
	
	lru.items = make(map[string]*list.Element)
	lru.order.Init()
}

func (lru *LRUEviction) Size() int {
	lru.mu.RLock()
	defer lru.mu.RUnlock()
	return len(lru.items)
}

// LFUEviction implements Least Frequently Used eviction
type LFUEviction struct {
	mu        sync.RWMutex
	capacity  int
	items     map[string]*lfuItem
	freqGroups map[int64]*list.List
	minFreq   int64
}

type lfuItem struct {
	key      string
	freq     int64
	size     int64
	element  *list.Element
}

// NewLFUEviction creates a new LFU eviction policy
func NewLFUEviction(capacity int) *LFUEviction {
	return &LFUEviction{
		capacity:   capacity,
		items:      make(map[string]*lfuItem),
		freqGroups: make(map[int64]*list.List),
		minFreq:    1,
	}
}

func (lfu *LFUEviction) OnAccess(key string) {
	lfu.mu.Lock()
	defer lfu.mu.Unlock()
	
	if item, exists := lfu.items[key]; exists {
		lfu.incrementFreq(item)
	}
}

func (lfu *LFUEviction) OnInsert(key string, size int64) {
	lfu.mu.Lock()
	defer lfu.mu.Unlock()
	
	if item, exists := lfu.items[key]; exists {
		item.size = size
		lfu.incrementFreq(item)
		return
	}
	
	item := &lfuItem{
		key:  key,
		freq: 1,
		size: size,
	}
	
	if lfu.freqGroups[1] == nil {
		lfu.freqGroups[1] = list.New()
	}
	
	item.element = lfu.freqGroups[1].PushFront(item)
	lfu.items[key] = item
	lfu.minFreq = 1
}

func (lfu *LFUEviction) OnDelete(key string) {
	lfu.mu.Lock()
	defer lfu.mu.Unlock()
	
	if item, exists := lfu.items[key]; exists {
		lfu.freqGroups[item.freq].Remove(item.element)
		delete(lfu.items, key)
		
		if lfu.freqGroups[lfu.minFreq].Len() == 0 {
			lfu.updateMinFreq()
		}
	}
}

func (lfu *LFUEviction) EvictOldest() string {
	lfu.mu.Lock()
	defer lfu.mu.Unlock()
	
	if len(lfu.items) == 0 {
		return ""
	}
	
	minList := lfu.freqGroups[lfu.minFreq]
	if minList == nil || minList.Len() == 0 {
		return ""
	}
	
	elem := minList.Back()
	if elem != nil {
		item := elem.Value.(*lfuItem)
		minList.Remove(elem)
		delete(lfu.items, item.key)
		
		if minList.Len() == 0 {
			lfu.updateMinFreq()
		}
		
		return item.key
	}
	
	return ""
}

func (lfu *LFUEviction) incrementFreq(item *lfuItem) {
	oldFreq := item.freq
	newFreq := oldFreq + 1
	
	// Remove from old frequency group
	lfu.freqGroups[oldFreq].Remove(item.element)
	
	// Add to new frequency group
	if lfu.freqGroups[newFreq] == nil {
		lfu.freqGroups[newFreq] = list.New()
	}
	
	item.freq = newFreq
	item.element = lfu.freqGroups[newFreq].PushFront(item)
	
	// Update minimum frequency if needed
	if lfu.freqGroups[lfu.minFreq].Len() == 0 {
		lfu.minFreq = newFreq
	}
}

func (lfu *LFUEviction) updateMinFreq() {
	for freq := lfu.minFreq; ; freq++ {
		if list, exists := lfu.freqGroups[freq]; exists && list.Len() > 0 {
			lfu.minFreq = freq
			break
		}
	}
}

func (lfu *LFUEviction) Clear() {
	lfu.mu.Lock()
	defer lfu.mu.Unlock()
	
	lfu.items = make(map[string]*lfuItem)
	lfu.freqGroups = make(map[int64]*list.List)
	lfu.minFreq = 1
}

func (lfu *LFUEviction) Size() int {
	lfu.mu.RLock()
	defer lfu.mu.RUnlock()
	return len(lfu.items)
}

// FIFOEviction implements First In First Out eviction
type FIFOEviction struct {
	mu    sync.RWMutex
	items *list.List
	keys  map[string]*list.Element
}

type fifoItem struct {
	key  string
	size int64
}

// NewFIFOEviction creates a new FIFO eviction policy
func NewFIFOEviction() *FIFOEviction {
	return &FIFOEviction{
		items: list.New(),
		keys:  make(map[string]*list.Element),
	}
}

func (fifo *FIFOEviction) OnAccess(key string) {
	// FIFO doesn't change order on access
}

func (fifo *FIFOEviction) OnInsert(key string, size int64) {
	fifo.mu.Lock()
	defer fifo.mu.Unlock()
	
	if elem, exists := fifo.keys[key]; exists {
		elem.Value.(*fifoItem).size = size
		return
	}
	
	item := &fifoItem{key: key, size: size}
	elem := fifo.items.PushBack(item)
	fifo.keys[key] = elem
}

func (fifo *FIFOEviction) OnDelete(key string) {
	fifo.mu.Lock()
	defer fifo.mu.Unlock()
	
	if elem, exists := fifo.keys[key]; exists {
		fifo.items.Remove(elem)
		delete(fifo.keys, key)
	}
}

func (fifo *FIFOEviction) EvictOldest() string {
	fifo.mu.Lock()
	defer fifo.mu.Unlock()
	
	if fifo.items.Len() == 0 {
		return ""
	}
	
	elem := fifo.items.Front()
	if elem != nil {
		item := elem.Value.(*fifoItem)
		fifo.items.Remove(elem)
		delete(fifo.keys, item.key)
		return item.key
	}
	
	return ""
}

func (fifo *FIFOEviction) Clear() {
	fifo.mu.Lock()
	defer fifo.mu.Unlock()
	
	fifo.items.Init()
	fifo.keys = make(map[string]*list.Element)
}

func (fifo *FIFOEviction) Size() int {
	fifo.mu.RLock()
	defer fifo.mu.RUnlock()
	return len(fifo.keys)
}

// CreateEvictionStrategy creates an eviction strategy based on policy
func CreateEvictionStrategy(policy types.EvictionPolicy, capacity int) EvictionStrategy {
	switch policy {
	case types.LRU:
		return NewLRUEviction(capacity)
	case types.LFU:
		return NewLFUEviction(capacity)
	case types.FIFO:
		return NewFIFOEviction()
	default:
		return NewLRUEviction(capacity)
	}
}