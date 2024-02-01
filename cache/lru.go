package cache

import (
	"container/list"
	"context"
	"sync"
	"sync/atomic"
	"time"
)

var _ Cache = (*LRUCache)(nil)

const LockStripeSize = 69

type (
	// EvictionCallBack is used to register a callback when a cache entry is evicted
	EvictionCallBack func(key string, value interface{})

	// Bucket is a container for expirable elements for O(1) removal
	Bucket map[string]*list.Element

	// Option is used to apply configurations to the cache
	Option func(*LRUCache)
)

type LRUCache struct {
	// The underlying kv map
	kv sync.Map

	// Optional: Capacity of the key-value storage
	cap atomic.Int64

	// Mutex for access to the lru linked list
	lm sync.RWMutex

	// Doubly linked list to keep track of the least recently used cache entries
	lru list.List

	// Optional: Default time-to-live for cache entries
	ttl atomic.Int64

	// Unix time bucketed expiry map of cache entries
	expiry sync.Map

	// Callback when a cache entry is evicted
	onEvict EvictionCallBack
}

func NewLRU(ctx context.Context, options ...Option) *LRUCache {
	lru := &LRUCache{}
	lru.cap.Store(-1)
	lru.ttl.Store(-1)
	for _, opt := range options {
		opt(lru)
	}

	lru.runGarbageCollection(ctx)
	return lru
}

func (lc *LRUCache) Get(key string) (Entry, bool) {
	if v, ok := lc.kv.Load(key); ok {
		element := v.(*list.Element)
		lc.lm.Lock()
		lc.lru.MoveToFront(element)
		lc.lm.Unlock()

		return element.Value.(*Item), true
	}

	return &Item{}, false
}

func (lc *LRUCache) Set(key string, value interface{}) bool {
	overwritten := lc.Delete(key)
	now := time.Now()
	var expiry time.Time
	if ttl := lc.DefaultTTL(); ttl > 0 {
		expiry = now.Add(ttl)
	}

	lc.lm.Lock()
	lc.kv.Store(key, lc.lru.PushFront(&Item{
		key:          key,
		value:        value,
		lastUpdated:  now,
		creationTime: now,
		expiryTime:   expiry,
	}))
	lc.lm.Unlock()

	if !expiry.IsZero() {
		timepoint := expiry.Unix()
		bucket, _ := lc.expiry.LoadOrStore(timepoint, new(sync.Map))
		bucket.(*sync.Map).Store(key, key)
	}

	return overwritten
}

func (lc *LRUCache) Update(key string, value interface{}) bool {
	if v, ok := lc.kv.Load(key); ok {
		element := v.(*list.Element)
		item := element.Value.(*Item)
		item.value = value
		item.lastUpdated = time.Now()
		lc.lm.Lock()
		lc.lru.MoveToFront(element)
		lc.lm.Unlock()

		return true
	}

	return false
}

func (lc *LRUCache) Delete(key string) bool {
	if v, ok := lc.kv.LoadAndDelete(key); ok {
		element := v.(*list.Element)
		if timepoint := element.Value.(*Item).ExpiryTime(); !timepoint.IsZero() {
			if bucket, ok := lc.expiry.Load(timepoint.Unix()); ok {
				bucket.(*sync.Map).Delete(key)
			}
		}

		lc.lm.Lock()
		lc.lru.Remove(element)
		lc.lm.Unlock()
		return true
	}

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
	if v, ok := lc.kv.Load(key); ok {
		return v.(*list.Element).Value.(*Item), true
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
	if cap <= 0 {
		cap = -1
	}
	lc.cap.Store(int64(cap))
}

func (lc *LRUCache) UpdateDefaultTTL(ttl time.Duration) {
	if ttl <= 0 {
		ttl = -1
	}

	lc.ttl.Store(ttl.Nanoseconds())
}

func (lc *LRUCache) DefaultTTL() time.Duration {
	return time.Duration(lc.ttl.Load())
}

func (lc *LRUCache) runGarbageCollection(ctx context.Context) {
	ticker := time.NewTicker(time.Second)

	callback := func(key, value interface{}) bool {
		lc.Delete(key.(string))
		return true
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				ticker.Stop()
				return

			case <-ticker.C:
				timepoint := time.Now().Unix() + 1
				if bucket, ok := lc.expiry.LoadAndDelete(timepoint); ok {
					bucket.(*sync.Map).Range(callback)
				}

			}
		}
	}()
}

func WithDefaultTTL(ttl time.Duration) Option {
	return func(lc *LRUCache) {
		if ttl > 0 {
			lc.ttl.Store(ttl.Nanoseconds())
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
