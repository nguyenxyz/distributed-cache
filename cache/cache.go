package cache

import (
	"container/list"
	"context"
	"sync"
	"sync/atomic"
	"time"
)

type Cache interface {
	// Get returns the entry for the given key if found
	Get(key string) (Entry, bool)

	// Set sets the value for the given key with the given ttl, overwrite if key exists, returns if key is overwritten
	Set(key string, value []byte, ttl time.Duration) bool

	// Update updates the value for the given key without resetting ttl, returns if key exists
	Update(key string, value []byte) bool

	// Delete removes the given key from the cache, returns if key was contained
	Delete(key string) bool

	// Purge removes all keys currently in the cache
	Purge()

	// Peek returns the entry for the given key if found without updating the cache's eviction policy
	Peek(key string) (Entry, bool)

	// Keys returns a slice of the keys in the cache
	Keys() []string

	// Entries returns a read-only slice of the entries in the cache
	Entries() []Entry

	// Size returns the number of entries in the cache
	Size() int64

	// Cap returns the current capacity of the cache
	Cap() int64

	// Resize resizes the cache with the provided capacity, overflowing entries will be evicted
	Resize(cap int64)

	// Recover recovers the cache from the given slice of entries, discards all previous entries
	Recover([]Entry)
}

type Entry interface {
	// Key returns the key associated with the entry
	Key() string

	// Value returns the value associated with the entry
	Value() []byte

	// LastUpdated returns the timestamp when the entry was last updated
	LastUpdated() time.Time

	// CreationTime returns the timestamp when the entry was first created
	CreationTime() time.Time

	// TTL returns the remaining time-to-live duration for the entry
	TTL() time.Duration

	// ExpiryTime returns the expiry time for the entry
	ExpiryTime() time.Time

	// Metadata returns the metadata associated with the entry
	Metadata() map[string]interface{}
}

type (
	// EvictionCallBack is used to register a callback when a cache entry is evicted
	EvictionCallBack func(key string, value []byte)

	// Option is used to apply configurations to the cache
	Option func(*MemoryCache)
)

func WithCapacity(cap int64) Option {
	return func(mc *MemoryCache) {
		if cap > 0 {
			mc.cap.Store(cap)
		}
	}
}

func WithEvictionCallback(cb EvictionCallBack) Option {
	return func(mc *MemoryCache) {
		mc.onEvict = cb
	}
}

func WithEvictionPolicy(policy EvictionPolicy) Option {
	return func(mc *MemoryCache) {
		mc.evictPolicy = policy
	}
}

type Operation int16

const (
	Get Operation = iota
	Set
	Update
	Delete
	Purge
)

var _ Cache = (MemoryCache)(nil)

type MemoryCache struct {
	// The underlying kv map
	kv sync.Map

	// Optional: Capacity of the cache
	cap atomic.Int64

	// Optional: Eviction policy for the cache
	evictPolicy EvictionPolicy

	// Unix time bucket for expirable keys in the cache
	timeBucket sync.Map

	// Callback when a cache entry is evicted
	onEvict EvictionCallBack
}

func NewMemoryCache(ctx context.Context, options ...Option) *MemoryCache {
	cache := &MemoryCache{}
	cache.cap.Store(-1)
	for _, opt := range options {
		opt(cache)
	}

	cache.runGarbageCollection(ctx)
	return cache
}

func (mc *MemoryCache) Get(key string) (Entry, bool) {
	if v, ok := mc.kv.Load(key); ok {
		mc.evictPolicy.register(Get, key)
		return v.(Item), true
	}

	return Item{}, false
}

func (mc *MemoryCache) Set(key string, value []byte, ttl time.Duration) bool {
	overwritten := mc.Delete(key)
	now := time.Now()
	var expiry time.Time
	if ttl > 0 {
		expiry = now.Add(ttl)
	}

	mc.evictPolicy.register(Set, key)
	if !expiry.IsZero() {
		mc.timeBucket.Add(expiry, key)
	}

	size, cap := mc.Size(), mc.Cap()
	evict := cap > 0 && size > cap
	if evict {
		mc.lm.Lock()
		defer mc.lm.Unlock()

		for size, cap := mc.lru.Len(), mc.Cap(); cap > 0 && int64(size) > cap; size, cap = mc.lru.Len(), mc.Cap() {
			element := mc.lru.Back()
			if _, ok := mc.kv.LoadAndDelete(element.Value.(Item).Key()); ok {
				item := element.Value.(Item)
				k, v := item.Key(), item.Value()
				if timepoint := item.ExpiryTime(); !timepoint.IsZero() {
					mc.expiry.Remove(timepoint, k)
				}
				mc.lru.Remove(element)
				if mc.onEvict != nil {
					mc.onEvict(k, v)
				}
			}

		}
	}

	return overwritten
}

func (mc *MemoryCache) Update(key string, value []byte) bool {
	if v, ok := mc.kv.Load(key); ok {
		element := v.(*list.Element)
		item := element.Value.(Item)
		item.value = value
		item.lastUpdated = time.Now()
		element.Value = item

		mc.lm.Lock()
		mc.lru.MoveToFront(element)
		mc.lm.Unlock()

		return true
	}

	return false
}

func (mc *MemoryCache) Delete(key string) bool {
	if v, ok := mc.kv.LoadAndDelete(key); ok {
		element := v.(*list.Element)
		if timepoint := element.Value.(Item).ExpiryTime(); !timepoint.IsZero() {
			mc.expiry.Remove(timepoint, key)
		}

		mc.lm.Lock()
		mc.lru.Remove(element)
		mc.lm.Unlock()

		return true
	}

	return false
}

func (mc *MemoryCache) Purge() {
	callback := func(k, v interface{}) bool {
		mc.Delete(k.(string))
		return true
	}

	mc.kv.Range(callback)
}

func (mc *MemoryCache) Peek(key string) (Entry, bool) {
	if v, ok := mc.kv.Load(key); ok {
		return v.(*list.Element).Value.(Item), true
	}

	return Item{}, false
}

func (mc *MemoryCache) Keys() []string {
	mc.lm.RLock()
	defer mc.lm.RUnlock()

	keys := make([]string, 0, mc.lru.Len())
	for e := mc.lru.Front(); e != nil; e = e.Next() {
		keys = append(keys, e.Value.(Item).Key())
	}

	return keys
}

func (mc *MemoryCache) Entries() []Entry {
	mc.lm.RLock()
	defer mc.lm.RUnlock()

	entries := make([]Entry, 0, mc.lru.Len())
	for e := mc.lru.Front(); e != nil; e = e.Next() {
		entries = append(entries, e.Value.(Item))
	}

	return entries
}

func (mc *MemoryCache) Size() int64 {
	mc.lm.RLock()
	defer mc.lm.RUnlock()

	return int64(mc.lru.Len())
}

func (mc *MemoryCache) Cap() int64 {
	return mc.cap.Load()
}

func (mc *MemoryCache) Resize(cap int64) {
	if cap <= 0 {
		cap = -1
	}
	mc.cap.Store(cap)
}

func (mc *MemoryCache) Recover(entries []Entry) {
	mc.Purge()
	for _, ent := range entries {
		if ent.TTL() != 0 {
			mc.kv.Store(ent.Key(), mc.lru.PushBack(Item{
				key:          ent.Key(),
				value:        ent.Value(),
				lastUpdated:  ent.LastUpdated(),
				creationTime: ent.CreationTime(),
				expiryTime:   ent.ExpiryTime(),
				metadata:     ent.Metadata(),
			}))
		}

		if expiry := ent.ExpiryTime(); !expiry.IsZero() {
			mc.expiry.Remove(expiry, ent.Key())
		}
	}
}

func (mc *MemoryCache) runGarbageCollection(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	callback := func(k, v interface{}) bool {
		go mc.Delete(k.(string))
		return true
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				ticker.Stop()
				return

			case <-ticker.C:
				mc.expiry.Prune(time.Now().Add(time.Second), callback)
			}
		}
	}()
}

func (tb *TimeBucket) Remove(timepoint time.Time, key string) {
	if expiryMap, ok := tb.m.Load(timepoint.Unix()); ok {
		expiryMap.(*sync.Map).Delete(key)
	}
}

func (tb *TimeBucket) Add(timepoint time.Time, key string) {
	expiryMap, _ := tb.m.LoadOrStore(timepoint.Unix(), new(sync.Map))
	expiryMap.(*sync.Map).Store(key, struct{}{})
}

func (tb *TimeBucket) Prune(timepoint time.Time, callback func(k, v interface{}) bool) {
	if expiryMap, ok := tb.m.LoadAndDelete(timepoint.Unix()); ok {
		expiryMap.(*sync.Map).Range(callback)
	}
}

var _ Entry = (*Item)(nil)

type Item struct {
	key          string
	value        []byte
	lastUpdated  time.Time
	creationTime time.Time
	expiryTime   time.Time
	metadata     map[string]interface{}
}

func (i Item) Key() string {
	return i.key
}

func (i Item) Value() []byte {
	return i.value
}

func (i Item) LastUpdated() time.Time {
	return i.lastUpdated
}

func (i Item) CreationTime() time.Time {
	return i.creationTime
}

func (i Item) TTL() time.Duration {
	if i.expiryTime.IsZero() {
		return -1
	}

	remaining := time.Until(i.expiryTime)
	if remaining < 0 {
		return 0
	}

	return remaining
}

func (i Item) ExpiryTime() time.Time {
	return i.expiryTime
}

func (i Item) Metadata() map[string]interface{} {
	return i.metadata
}
