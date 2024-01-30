package cache

import (
	"container/list"
	"context"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

var _ Cache = (*LRUCache)(nil)

// EvictionCallBack is used to register a callback when a cache entry is evicted
type EvictionCallBack func(key string, value interface{})

// Bucket is a container for expirable elements for O(1) removal
type Bucket map[string]*list.Element

type LRUCache struct {
	// The underlying kv map
	kv map[string]*list.Element

	// Adaptive lock manager for concurrent access to any key in key space in kv map
	kvLockManager AdaptiveLockManager

	// Optional: Capacity of the key-value storage
	cap atomic.Int64

	// Lock for thread-safe access to the lru linked list
	lm sync.RWMutex

	// Doubly linked list to keep track of the least recently used cache entries
	lru list.List

	// Optional: Default time-to-live for cache entries
	ttl time.Duration

	// Unix time bucketed expiry map of cache entries
	expiry map[int64]Bucket

	// Lock manager for concurrent access to subsets of key space in the expiry map
	expiryLockManager LockManager

	// Callback when a cache entry is evicted
	onEvict EvictionCallBack
}

func NewLRU(ctx context.Context, options ...Option) *LRUCache {
	lru := &LRUCache{
		kv:                make(map[string]*list.Element),
		kvLockManager:     AdaptiveLockManager{},
		expiry:            make(map[int64]Bucket),
		expiryLockManager: LockManager{},
		ttl:               -1,
	}

	lru.cap.Store(-1)
	for _, opt := range options {
		opt(lru)
	}

	if lru.ttl > 0 {
		lru.runGarbageCollection(ctx)
	}

	return lru
}

func (lc *LRUCache) Get(key string) (Entry, bool) {
	lock := lc.kvLockManager.Get(key)
	lock.RLock()
	defer lock.RUnlock()

	if e, ok := lc.kv[key]; ok {
		lc.lm.Lock()
		lc.lru.MoveToFront(e)
		lc.lm.Unlock()

		return e.Value.(*Item), true
	}

	return &Item{}, false
}

func (lc *LRUCache) Set(key string, value interface{}) bool {
	lock := lc.kvLockManager.Get(key)
	lock.Lock()
	defer lock.Unlock()

	if e, ok := lc.kv[key]; ok {
		timepoint := e.Value.(*Item).CreationTime().Add(lc.ttl).Unix()
		lc.removeFromExpiryBucket(timepoint, key)
		lc.lm.Lock()
		lc.lru.Remove(e)
		lc.lm.Unlock()
	}

	now := time.Now()
	item := &Item{
		key:          key,
		value:        value,
		lastUpdated:  now,
		creationTime: now,
		setTTL:       lc.ttl,
	}

	lc.lm.Lock()
	lc.kv[key] = lc.lru.PushFront(item)
	cap := lc.Cap()
	evict := cap > 0 && lc.lru.Len() > cap
	for evict && lc.lru.Len() > lc.Cap() {
		entry := lc.lru.Back()
		lc.lru.Remove(entry)
		key, value := entry.Value.(*Item).Key(), entry.Value.(*Item).Value()
		lock := lc.kvLockManager.Get(key)
		lock.Lock()
		delete(lc.kv, key)
		lock.Unlock()

		if lc.onEvict != nil {
			go lc.onEvict(key, value)
		}
	}
	lc.lm.Unlock()

	if lc.ttl > 0 {
		timepoint := now.Add(lc.ttl).Unix()
		el := lc.expiryLockManager.Get(strconv.Itoa(int(timepoint)))
		el.Lock()
		lc.expiry[timepoint][key] = lc.kv[key]
		el.Unlock()
	}

	return evict
}

func (lc *LRUCache) Update(key string, value interface{}) bool {
	lock := lc.kvLockManager.Get(key)
	lock.Lock()
	defer lock.Unlock()

	if e, ok := lc.kv[key]; ok {
		item := e.Value.(*Item)
		item.value = value
		item.lastUpdated = time.Now()

		return true
	}

	return false
}

func (lc *LRUCache) Delete(key string) bool {
	lock := lc.kvLockManager.Get(key)
	lock.Lock()
	defer lock.Unlock()

	if e, ok := lc.kv[key]; ok {
		lc.lm.Lock()
		lc.lru.Remove(e)
		lc.lm.Unlock()
		delete(lc.kv, key)

		return true
	}

	lc.kvLockManager.Remove(key)
	return false
}

func (lc *LRUCache) Purge() {
	keys := lc.Keys()
	var wg sync.WaitGroup

	for _, k := range keys {
		wg.Add(1)

		go func(key string) {
			defer wg.Done()

			lc.Delete(key)
		}(k)
	}

	wg.Wait()
}

func (lc *LRUCache) Peek(key string) (Entry, bool) {
	lock := lc.kvLockManager.Get(key)
	lock.RLock()
	defer lock.RUnlock()

	if e, ok := lc.kv[key]; ok {
		return e.Value.(*Item), true
	}

	return &Item{}, false
}

func (lc *LRUCache) Keys() []string {
	lc.lm.RLock()
	defer lc.lm.RUnlock()

	keys := make([]string, 0, lc.lru.Len())
	for e := lc.lru.Front(); e != nil; e = e.Next() {
		keys = append(keys, e.Value.(*Item).Key())
	}

	return keys
}

func (lc *LRUCache) Entries() []Entry {
	lc.lm.RLock()
	defer lc.lm.RUnlock()

	entries := make([]Entry, 0, lc.lru.Len())
	for e := lc.lru.Front(); e != nil; e = e.Next() {
		entries = append(entries, e.Value.(*Item))
	}

	return entries
}

func (lc *LRUCache) Len() int {
	lc.lm.RLock()
	defer lc.lm.RUnlock()

	return lc.lru.Len()
}

func (lc *LRUCache) Cap() int {
	return int(lc.cap.Load())
}

func (lc *LRUCache) Resize(cap int) {
	lc.cap.Store(int64(cap))
}

func (lc *LRUCache) removeFromExpiryBucket(timepoint int64, key string) {
	lock := lc.expiryLockManager.Get(strconv.Itoa(int(timepoint)))
	lock.Lock()
	defer lock.Unlock()

	delete(lc.expiry[timepoint], key)
}

func (lc *LRUCache) runGarbageCollection(ctx context.Context) {
	ticker := time.NewTicker(time.Second)

	go func() {
		for {
			select {
			case <-ctx.Done():
				ticker.Stop()
				return

			case <-ticker.C:
				timepoint := time.Now().Unix() + 1
				lock := lc.expiryLockManager.Get(strconv.Itoa(int(timepoint)))
				lock.Lock()
				if l, ok := lc.expiry[timepoint]; ok {
					for _, e := range l {
						go lc.Delete(e.Value.(*Item).Key())
					}
					delete(lc.expiry, timepoint)
				}
				lock.Unlock()
			}
		}
	}()
}

type Option func(*LRUCache)

func WithDefaultTTL(ttl time.Duration) Option {
	return func(lc *LRUCache) {
		if ttl > 0 {
			lc.ttl = ttl
		}
	}
}

func WithCapacity(cap int) Option {
	return func(lc *LRUCache) {
		if cap > 0 {
			lc.cap.Store(int64(cap))
		}
	}
}

func WithEvictionCallback(cb EvictionCallBack) Option {
	return func(lc *LRUCache) {
		lc.onEvict = cb
	}
}
