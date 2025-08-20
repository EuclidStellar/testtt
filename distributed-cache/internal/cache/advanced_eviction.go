package cache

import (
	"container/list"
	"sync"
	"time"
)

// LRUKEviction implements LRU-K eviction policy
// Tracks K most recent accesses for each item
type LRUKEviction struct {
	mu            sync.RWMutex
	capacity      int
	k             int // Number of access history to track
	historyQueue  *list.List
	cacheQueue    *list.List
	historyMap    map[string]*list.Element
	cacheMap      map[string]*list.Element
}

type lrukItem struct {
	key         string
	size        int64
	accessTimes []time.Time
}

// NewLRUKEviction creates a new LRU-K eviction policy
func NewLRUKEviction(capacity, k int) *LRUKEviction {
	return &LRUKEviction{
		capacity:     capacity,
		k:            k,
		historyQueue: list.New(),
		cacheQueue:   list.New(),
		historyMap:   make(map[string]*list.Element),
		cacheMap:     make(map[string]*list.Element),
	}
}

func (lk *LRUKEviction) OnAccess(key string) {
	lk.mu.Lock()
	defer lk.mu.Unlock()

	now := time.Now()

	// Check if in cache
	if elem, exists := lk.cacheMap[key]; exists {
		item := elem.Value.(*lrukItem)
		item.accessTimes = append(item.accessTimes, now)
		if len(item.accessTimes) > lk.k {
			item.accessTimes = item.accessTimes[1:]
		}
		lk.cacheQueue.MoveToFront(elem)
		return
	}

	// Check if in history
	if elem, exists := lk.historyMap[key]; exists {
		item := elem.Value.(*lrukItem)
		item.accessTimes = append(item.accessTimes, now)
		if len(item.accessTimes) >= lk.k {
			// Promote to cache
			lk.historyQueue.Remove(elem)
			delete(lk.historyMap, key)
			
			newElem := lk.cacheQueue.PushFront(item)
			lk.cacheMap[key] = newElem
		} else {
			lk.historyQueue.MoveToFront(elem)
		}
	}
}

func (lk *LRUKEviction) OnInsert(key string, size int64) {
	lk.mu.Lock()
	defer lk.mu.Unlock()

	now := time.Now()
	item := &lrukItem{
		key:         key,
		size:        size,
		accessTimes: []time.Time{now},
	}

	// Add to history first
	elem := lk.historyQueue.PushFront(item)
	lk.historyMap[key] = elem
}

func (lk *LRUKEviction) OnDelete(key string) {
	lk.mu.Lock()
	defer lk.mu.Unlock()

	if elem, exists := lk.cacheMap[key]; exists {
		lk.cacheQueue.Remove(elem)
		delete(lk.cacheMap, key)
		return
	}

	if elem, exists := lk.historyMap[key]; exists {
		lk.historyQueue.Remove(elem)
		delete(lk.historyMap, key)
	}
}

func (lk *LRUKEviction) EvictOldest() string {
	lk.mu.Lock()
	defer lk.mu.Unlock()

	// First try to evict from cache
	if lk.cacheQueue.Len() > 0 {
		elem := lk.cacheQueue.Back()
		item := elem.Value.(*lrukItem)
		lk.cacheQueue.Remove(elem)
		delete(lk.cacheMap, item.key)
		return item.key
	}

	// Then from history
	if lk.historyQueue.Len() > 0 {
		elem := lk.historyQueue.Back()
		item := elem.Value.(*lrukItem)
		lk.historyQueue.Remove(elem)
		delete(lk.historyMap, item.key)
		return item.key
	}

	return ""
}

func (lk *LRUKEviction) Clear() {
	lk.mu.Lock()
	defer lk.mu.Unlock()
	
	lk.historyQueue.Init()
	lk.cacheQueue.Init()
	lk.historyMap = make(map[string]*list.Element)
	lk.cacheMap = make(map[string]*list.Element)
}

func (lk *LRUKEviction) Size() int {
	lk.mu.RLock()
	defer lk.mu.RUnlock()
	return len(lk.historyMap) + len(lk.cacheMap)
}

// TwoQEviction implements 2Q eviction policy
// Maintains hot queue (LRU) and cold queue (FIFO)
type TwoQEviction struct {
	mu         sync.RWMutex
	capacity   int
	hotSize    int
	coldSize   int
	hotQueue   *list.List
	coldQueue  *list.List
	ghostQueue *list.List
	hotMap     map[string]*list.Element
	coldMap    map[string]*list.Element
	ghostMap   map[string]*list.Element
}

type twoQItem struct {
	key  string
	size int64
}

// NewTwoQEviction creates a new 2Q eviction policy
func NewTwoQEviction(capacity int) *TwoQEviction {
	hotSize := capacity * 3 / 4   // 75% for hot
	coldSize := capacity / 4      // 25% for cold

	return &TwoQEviction{
		capacity:   capacity,
		hotSize:    hotSize,
		coldSize:   coldSize,
		hotQueue:   list.New(),
		coldQueue:  list.New(),
		ghostQueue: list.New(),
		hotMap:     make(map[string]*list.Element),
		coldMap:    make(map[string]*list.Element),
		ghostMap:   make(map[string]*list.Element),
	}
}

func (tq *TwoQEviction) OnAccess(key string) {
	tq.mu.Lock()
	defer tq.mu.Unlock()

	// Check hot queue
	if elem, exists := tq.hotMap[key]; exists {
		tq.hotQueue.MoveToFront(elem)
		return
	}

	// Check cold queue - promote to hot
	if elem, exists := tq.coldMap[key]; exists {
		item := elem.Value.(*twoQItem)
		tq.coldQueue.Remove(elem)
		delete(tq.coldMap, key)

		// Add to hot queue
		hotElem := tq.hotQueue.PushFront(item)
		tq.hotMap[key] = hotElem

		// Evict from hot if necessary
		if tq.hotQueue.Len() > tq.hotSize {
			tq.evictFromHot()
		}
		return
	}

	// Check ghost queue - promote to hot
	if elem, exists := tq.ghostMap[key]; exists {
		tq.ghostQueue.Remove(elem)
		delete(tq.ghostMap, key)
		// Will be re-inserted as new access
	}
}

func (tq *TwoQEviction) OnInsert(key string, size int64) {
	tq.mu.Lock()
	defer tq.mu.Unlock()

	item := &twoQItem{key: key, size: size}

	// Check if it was in ghost (second chance)
	if _, exists := tq.ghostMap[key]; exists {
		// Promote directly to hot
		delete(tq.ghostMap, key)
		elem := tq.hotQueue.PushFront(item)
		tq.hotMap[key] = elem

		if tq.hotQueue.Len() > tq.hotSize {
			tq.evictFromHot()
		}
	} else {
		// Add to cold queue (first access)
		elem := tq.coldQueue.PushFront(item)
		tq.coldMap[key] = elem

		if tq.coldQueue.Len() > tq.coldSize {
			tq.evictFromCold()
		}
	}
}

func (tq *TwoQEviction) OnDelete(key string) {
	tq.mu.Lock()
	defer tq.mu.Unlock()

	if elem, exists := tq.hotMap[key]; exists {
		tq.hotQueue.Remove(elem)
		delete(tq.hotMap, key)
		return
	}

	if elem, exists := tq.coldMap[key]; exists {
		tq.coldQueue.Remove(elem)
		delete(tq.coldMap, key)
		return
	}

	if elem, exists := tq.ghostMap[key]; exists {
		tq.ghostQueue.Remove(elem)
		delete(tq.ghostMap, key)
	}
}

func (tq *TwoQEviction) EvictOldest() string {
	tq.mu.Lock()
	defer tq.mu.Unlock()

	// Try cold first
	if tq.coldQueue.Len() > 0 {
		return tq.evictFromCold()
	}

	// Then hot
	if tq.hotQueue.Len() > 0 {
		return tq.evictFromHot()
	}

	return ""
}

func (tq *TwoQEviction) evictFromCold() string {
	if tq.coldQueue.Len() == 0 {
		return ""
	}

	elem := tq.coldQueue.Back()
	item := elem.Value.(*twoQItem)
	tq.coldQueue.Remove(elem)
	delete(tq.coldMap, item.key)

	// Add to ghost queue
	ghostElem := tq.ghostQueue.PushFront(&twoQItem{key: item.key, size: item.size})
	tq.ghostMap[item.key] = ghostElem

	// Maintain ghost queue size
	if tq.ghostQueue.Len() > tq.capacity {
		backElem := tq.ghostQueue.Back()
		backItem := backElem.Value.(*twoQItem)
		tq.ghostQueue.Remove(backElem)
		delete(tq.ghostMap, backItem.key)
	}

	return item.key
}

func (tq *TwoQEviction) evictFromHot() string {
	if tq.hotQueue.Len() == 0 {
		return ""
	}

	elem := tq.hotQueue.Back()
	item := elem.Value.(*twoQItem)
	tq.hotQueue.Remove(elem)
	delete(tq.hotMap, item.key)
	return item.key
}

func (tq *TwoQEviction) Clear() {
	tq.mu.Lock()
	defer tq.mu.Unlock()

	tq.hotQueue.Init()
	tq.coldQueue.Init()
	tq.ghostQueue.Init()
	tq.hotMap = make(map[string]*list.Element)
	tq.coldMap = make(map[string]*list.Element)
	tq.ghostMap = make(map[string]*list.Element)
}

func (tq *TwoQEviction) Size() int {
	tq.mu.RLock()
	defer tq.mu.RUnlock()
	return len(tq.hotMap) + len(tq.coldMap)
}

// ARCEviction implements Adaptive Replacement Cache
// Balances between LRU and LFU based on workload
type ARCEviction struct {
	mu     sync.RWMutex
	c      int // Target size
	p      int // Adaptation parameter
	t1     *list.List // Recent pages
	t2     *list.List // Frequent pages  
	b1     *list.List // Ghost entries evicted from t1
	b2     *list.List // Ghost entries evicted from t2
	t1Map  map[string]*list.Element
	t2Map  map[string]*list.Element
	b1Map  map[string]*list.Element
	b2Map  map[string]*list.Element
}

type arcItem struct {
	key  string
	size int64
}

// NewARCEviction creates a new ARC eviction policy
func NewARCEviction(capacity int) *ARCEviction {
	return &ARCEviction{
		c:     capacity,
		p:     0,
		t1:    list.New(),
		t2:    list.New(),
		b1:    list.New(),
		b2:    list.New(),
		t1Map: make(map[string]*list.Element),
		t2Map: make(map[string]*list.Element),
		b1Map: make(map[string]*list.Element),
		b2Map: make(map[string]*list.Element),
	}
}

func (arc *ARCEviction) OnAccess(key string) {
	arc.mu.Lock()
	defer arc.mu.Unlock()

	// Case 1: x in T1
	if elem, exists := arc.t1Map[key]; exists {
		// Move to T2
		item := elem.Value.(*arcItem)
		arc.t1.Remove(elem)
		delete(arc.t1Map, key)
		
		newElem := arc.t2.PushFront(item)
		arc.t2Map[key] = newElem
		return
	}

	// Case 2: x in T2  
	if elem, exists := arc.t2Map[key]; exists {
		arc.t2.MoveToFront(elem)
		return
	}
}

func (arc *ARCEviction) OnInsert(key string, size int64) {
	arc.mu.Lock()
	defer arc.mu.Unlock()

	item := &arcItem{key: key, size: size}

	// Case 1: x in B1
	if elem, exists := arc.b1Map[key]; exists {
		// Adapt: increase p
		delta := 1
		if len(arc.b2Map) > len(arc.b1Map) {
			delta = len(arc.b2Map) / len(arc.b1Map)
		}
		arc.p = min(arc.c, arc.p + delta)

		// Replace and move to T2
		arc.replace(key)
		arc.b1.Remove(elem)
		delete(arc.b1Map, key)
		
		newElem := arc.t2.PushFront(item)
		arc.t2Map[key] = newElem
		return
	}

	// Case 2: x in B2
	if elem, exists := arc.b2Map[key]; exists {
		// Adapt: decrease p
		delta := 1
		if len(arc.b1Map) > len(arc.b2Map) {
			delta = len(arc.b1Map) / len(arc.b2Map)
		}
		arc.p = max(0, arc.p - delta)

		// Replace and move to T2
		arc.replace(key)
		arc.b2.Remove(elem)
		delete(arc.b2Map, key)
		
		newElem := arc.t2.PushFront(item)
		arc.t2Map[key] = newElem
		return
	}

	// Case 3: x not in cache
	if len(arc.t1Map) + len(arc.b1Map) == arc.c {
		if len(arc.t1Map) < arc.c {
			// Delete LRU page in B1 and replace
			if arc.b1.Len() > 0 {
				lruElem := arc.b1.Back()
				lruItem := lruElem.Value.(*arcItem)
				arc.b1.Remove(lruElem)
				delete(arc.b1Map, lruItem.key)
			}
			arc.replace(key)
		} else {
			// Delete LRU page in T1
			if arc.t1.Len() > 0 {
				lruElem := arc.t1.Back()
				lruItem := lruElem.Value.(*arcItem)
				arc.t1.Remove(lruElem)
				delete(arc.t1Map, lruItem.key)
			}
		}
	} else if len(arc.t1Map) + len(arc.t2Map) + len(arc.b1Map) + len(arc.b2Map) >= arc.c {
		if len(arc.t1Map) + len(arc.t2Map) + len(arc.b1Map) + len(arc.b2Map) == 2 * arc.c {
			// Delete LRU page in B2
			if arc.b2.Len() > 0 {
				lruElem := arc.b2.Back()
				lruItem := lruElem.Value.(*arcItem)
				arc.b2.Remove(lruElem)
				delete(arc.b2Map, lruItem.key)
			}
		}
		arc.replace(key)
	}

	// Insert x at the top of T1
	newElem := arc.t1.PushFront(item)
	arc.t1Map[key] = newElem
}

func (arc *ARCEviction) replace(key string) {
	if len(arc.t1Map) != 0 && ((len(arc.t1Map) > arc.p) || (len(arc.t1Map) == arc.p && len(arc.b2Map) > 0)) {
		// Move LRU in T1 to B1
		if arc.t1.Len() > 0 {
			lruElem := arc.t1.Back()
			lruItem := lruElem.Value.(*arcItem)
			arc.t1.Remove(lruElem)
			delete(arc.t1Map, lruItem.key)
			
			b1Elem := arc.b1.PushFront(&arcItem{key: lruItem.key, size: lruItem.size})
			arc.b1Map[lruItem.key] = b1Elem
		}
	} else {
		// Move LRU in T2 to B2
		if arc.t2.Len() > 0 {
			lruElem := arc.t2.Back()
			lruItem := lruElem.Value.(*arcItem)
			arc.t2.Remove(lruElem)
			delete(arc.t2Map, lruItem.key)
			
			b2Elem := arc.b2.PushFront(&arcItem{key: lruItem.key, size: lruItem.size})
			arc.b2Map[lruItem.key] = b2Elem
		}
	}
}

func (arc *ARCEviction) OnDelete(key string) {
	arc.mu.Lock()
	defer arc.mu.Unlock()

	if elem, exists := arc.t1Map[key]; exists {
		arc.t1.Remove(elem)
		delete(arc.t1Map, key)
		return
	}

	if elem, exists := arc.t2Map[key]; exists {
		arc.t2.Remove(elem)
		delete(arc.t2Map, key)
		return
	}

	if elem, exists := arc.b1Map[key]; exists {
		arc.b1.Remove(elem)
		delete(arc.b1Map, key)
		return
	}

	if elem, exists := arc.b2Map[key]; exists {
		arc.b2.Remove(elem)
		delete(arc.b2Map, key)
	}
}

func (arc *ARCEviction) EvictOldest() string {
	arc.mu.Lock()
	defer arc.mu.Unlock()

	// Evict from T1 first
	if arc.t1.Len() > 0 {
		elem := arc.t1.Back()
		item := elem.Value.(*arcItem)
		arc.t1.Remove(elem)
		delete(arc.t1Map, item.key)
		
		// Move to B1
		b1Elem := arc.b1.PushFront(&arcItem{key: item.key, size: item.size})
		arc.b1Map[item.key] = b1Elem
		
		return item.key
	}

	// Then from T2
	if arc.t2.Len() > 0 {
		elem := arc.t2.Back()
		item := elem.Value.(*arcItem)
		arc.t2.Remove(elem)
		delete(arc.t2Map, item.key)
		
		// Move to B2
		b2Elem := arc.b2.PushFront(&arcItem{key: item.key, size: item.size})
		arc.b2Map[item.key] = b2Elem
		
		return item.key
	}

	return ""
}

func (arc *ARCEviction) Clear() {
	arc.mu.Lock()
	defer arc.mu.Unlock()

	arc.t1.Init()
	arc.t2.Init()
	arc.b1.Init()
	arc.b2.Init()
	arc.t1Map = make(map[string]*list.Element)
	arc.t2Map = make(map[string]*list.Element)
	arc.b1Map = make(map[string]*list.Element)
	arc.b2Map = make(map[string]*list.Element)
	arc.p = 0
}

func (arc *ARCEviction) Size() int {
	arc.mu.RLock()
	defer arc.mu.RUnlock()
	return len(arc.t1Map) + len(arc.t2Map)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
