package cache

import (
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
	EvictionCallBack func(key string)

	// Option is used to apply configurations to the cache
	Option func(*MemoryCache)

	// Unix time bucket is used by garbage collector to manage expirable keys
	UnixTimeBucket struct {
		m sync.Map
	}
)

var _ Cache = (*MemoryCache)(nil)

type MemoryCache struct {
	// The underlying kv map
	kvmap sync.Map

	// The current size of the cache
	size atomic.Int64

	// Optional: Capacity of the cache
	cap atomic.Int64

	// Optional: Eviction policy for the cache
	evictPolicy EvictionPolicy

	// Unix time bucket for expirable keys in the cache
	unixTimeBucket UnixTimeBucket

	// Callback when a cache entry is evicted
	onEvict EvictionCallBack
}

func NewMemoryCache(ctx context.Context, options ...Option) Cache {
	cache := &MemoryCache{}
	cache.cap.Store(-1)
	for _, opt := range options {
		opt(cache)
	}

	cache.runGarbageCollection(ctx)
	return cache
}

func (memc *MemoryCache) Get(key string) (Entry, bool) {
	if v, ok := memc.kvmap.Load(key); ok {
		memc.evictPolicy.Register(Get, key)
		return v.(Item), true
	}

	return Item{}, false
}

func (memc *MemoryCache) Set(key string, value []byte, ttl time.Duration) bool {
	memc.Delete(key)
	now := time.Now()
	var expiry time.Time
	if ttl > 0 {
		expiry = now.Add(ttl)
	}

	_, loaded := memc.kvmap.LoadOrStore(key, Item{
		key:          key,
		value:        value,
		lastUpdated:  now,
		creationTime: now,
		expiryTime:   expiry,
	})
	if !loaded {
		memc.evictPolicy.Register(Set, key)
		memc.size.Add(1)
		if !expiry.IsZero() {
			memc.unixTimeBucket.Add(expiry, key)
		}
	}

	// start eviction if cache exceeds capacity
	shouldEvict := func() bool {
		size, cap := memc.Size(), memc.Cap()
		return cap > 0 && size > cap
	}

	for shouldEvict() {
		evictKey, ok := memc.evictPolicy.Next()
		if !ok {
			// Cache is over capacity, but the eviction policy has no keys to evict.
			// This might indicate an inconsistency between the cache's size
			// tracking and the policy's tracked keys.
			break
		}

		memc.Delete(evictKey)
		if memc.onEvict != nil {
			memc.onEvict(evictKey)
		}
	}

	return !loaded
}

func (memc *MemoryCache) Update(key string, value []byte) bool {
	if v, ok := memc.kvmap.Load(key); ok {
		old := v.(Item)
		memc.kvmap.Store(key, Item{
			key:          key,
			value:        value,
			lastUpdated:  time.Now(),
			creationTime: old.CreationTime(),
			expiryTime:   old.ExpiryTime(),
			metadata:     old.Metadata(),
		})
		memc.evictPolicy.Register(Update, key)
		return true
	}

	return false
}

func (memc *MemoryCache) Delete(key string) bool {
	if v, ok := memc.kvmap.LoadAndDelete(key); ok {
		old := v.(Item)
		if expiry := old.ExpiryTime(); !expiry.IsZero() {
			memc.unixTimeBucket.Remove(expiry, key)
		}
		memc.evictPolicy.Register(Delete, key)
		memc.size.Add(-1)
		return true
	}

	return false
}

func (memc *MemoryCache) Purge() {
	memc.kvmap.Clear()
	memc.evictPolicy.Reset()
	memc.size.Store(0)
	memc.unixTimeBucket.Clear()
}

func (memc *MemoryCache) Peek(key string) (Entry, bool) {
	if v, ok := memc.kvmap.Load(key); ok {
		return v.(Item), true
	}

	return Item{}, false
}

func (memc *MemoryCache) Keys() []string {
	keys := make([]string, 0, memc.Size())
	memc.kvmap.Range(func(k, v interface{}) bool {
		keys = append(keys, k.(string))
		return true
	})
	return keys
}

func (memc *MemoryCache) Entries() []Entry {
	entries := make([]Entry, 0, memc.Size())
	memc.kvmap.Range(func(k, v interface{}) bool {
		entries = append(entries, v.(Item))
		return true
	})
	return entries
}

func (memc *MemoryCache) Size() int64 {
	return memc.size.Load()
}

func (memc *MemoryCache) Cap() int64 {
	return memc.cap.Load()
}

func (memc *MemoryCache) Resize(cap int64) {
	if cap <= 0 {
		cap = -1
	}
	memc.cap.Store(cap)
}

func (memc *MemoryCache) Recover(entries []Entry) {
	memc.Purge()
	for _, entry := range entries[:min(int64(len(entries)), memc.Cap())] {
		if entry.TTL() != 0 {
			memc.kvmap.Store(entry.Key(), Item{
				key:          entry.Key(),
				value:        entry.Value(),
				lastUpdated:  entry.LastUpdated(),
				creationTime: entry.CreationTime(),
				expiryTime:   entry.ExpiryTime(),
				metadata:     entry.Metadata(),
			})
			memc.evictPolicy.Register(Set, entry.Key())
			memc.unixTimeBucket.Add(entry.ExpiryTime(), entry.Key())
		}
	}
}

func (memc *MemoryCache) runGarbageCollection(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	go func() {
		for {
			select {
			case <-ctx.Done():
				ticker.Stop()
				return

			case <-ticker.C:
				memc.unixTimeBucket.Prune(time.Now().Add(time.Second), func(k, v interface{}) bool {
					go memc.Delete(k.(string))
					return true
				})
			}
		}
	}()
}

func WithCapacity(cap int64) Option {
	return func(memc *MemoryCache) {
		if cap > 0 {
			memc.cap.Store(cap)
		}
	}
}

func WithEvictionCallback(cb EvictionCallBack) Option {
	return func(memc *MemoryCache) {
		memc.onEvict = cb
	}
}

func WithEvictionPolicy(policy EvictionPolicy) Option {
	return func(memc *MemoryCache) {
		memc.evictPolicy = policy
	}
}

func (utb *UnixTimeBucket) Remove(timepoint time.Time, key string) {
	if expiryMap, ok := utb.m.Load(timepoint.Unix()); ok {
		expiryMap.(*sync.Map).Delete(key)
	}
}

func (utb *UnixTimeBucket) Add(timepoint time.Time, key string) {
	expiryMap, _ := utb.m.LoadOrStore(timepoint.Unix(), new(sync.Map))
	expiryMap.(*sync.Map).Store(key, struct{}{})
}

func (utb *UnixTimeBucket) Prune(timepoint time.Time, callback func(k, v interface{}) bool) {
	if expiryMap, ok := utb.m.LoadAndDelete(timepoint.Unix()); ok {
		expiryMap.(*sync.Map).Range(callback)
	}
}

func (utb *UnixTimeBucket) Clear() {
	utb.m.Clear()
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
