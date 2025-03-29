package cache

import (
	"context"
	"sync"
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
	Size() int

	// Cap returns the current capacity of the cache
	Cap() int

	// Resize resizes the cache with the provided capacity, overflowing entries will be evicted
	Resize(cap int)

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
		timeToBucket sync.Map
	}
)

var _ Cache = (*MemoryCache)(nil)

type MemoryCache struct {
	// Mutex to protect the cache for concurrent access
	mu sync.RWMutex

	// The underlying kv map
	kv map[string]Item

	// Optional: Capacity of the cache
	cap int

	// Optional: Eviction policy for the cache
	evictPolicy EvictionPolicy

	// Unix time bucket for expirable keys in the cache
	unixTimeBucket UnixTimeBucket

	// Callback when a cache entry is evicted
	onEvict EvictionCallBack
}

func NewMemoryCache(ctx context.Context, options ...Option) Cache {
	cache := &MemoryCache{
		kv:  make(map[string]Item),
		cap: -1,
	}
	for _, opt := range options {
		opt(cache)
	}

	if cache.evictPolicy == nil {
		cache.evictPolicy = NewLRUPolicy()
	}

	cache.runGarbageCollection(ctx)
	return cache
}

func (memc *MemoryCache) Get(key string) (Entry, bool) {
	memc.mu.RLock()
	defer memc.mu.RUnlock()

	if item, ok := memc.kv[key]; ok {
		memc.evictPolicy.Register(Get, key)
		return item, true
	}

	return Item{}, false
}

func (memc *MemoryCache) Set(key string, value []byte, ttl time.Duration) bool {
	memc.mu.Lock()
	defer memc.mu.Unlock()

	now := time.Now()
	var expiry time.Time
	if ttl > 0 {
		expiry = now.Add(ttl)
	}

	deleted := memc.deleteInternal(key)
	memc.kv[key] = Item{
		key:          key,
		value:        value,
		lastUpdated:  now,
		creationTime: now,
		expiryTime:   expiry,
	}

	memc.evictPolicy.Register(Set, key)
	if !expiry.IsZero() {
		memc.unixTimeBucket.AddKeyToBucket(expiry, key)
	}

	memc.evictIfNeeded()
	return !deleted
}

func (memc *MemoryCache) evictIfNeeded() {
	shouldEvict := func() bool {
		size, cap := len(memc.kv), memc.cap
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

		memc.deleteInternal(evictKey)
		if memc.onEvict != nil {
			memc.onEvict(evictKey)
		}
	}
}

func (memc *MemoryCache) Update(key string, value []byte) bool {
	memc.mu.Lock()
	defer memc.mu.Unlock()

	if stale, ok := memc.kv[key]; ok {
		memc.kv[key] = Item{
			key:          key,
			value:        value,
			lastUpdated:  time.Now(),
			creationTime: stale.CreationTime(),
			expiryTime:   stale.ExpiryTime(),
			metadata:     stale.Metadata(),
		}
		memc.evictPolicy.Register(Update, key)
		return true
	}

	return false
}

func (memc *MemoryCache) Delete(key string) bool {
	memc.mu.Lock()
	defer memc.mu.Unlock()
	return memc.deleteInternal(key)
}

func (memc *MemoryCache) deleteInternal(key string) bool {
	if item, ok := memc.kv[key]; ok {
		if expiry := item.ExpiryTime(); !expiry.IsZero() {
			memc.unixTimeBucket.RemoveKeyFromBucket(expiry, key)
		}
		delete(memc.kv, key)
		memc.evictPolicy.Register(Delete, key)
		return true
	}

	return false
}

func (memc *MemoryCache) Purge() {
	memc.mu.Lock()
	defer memc.mu.Unlock()

	memc.kv = make(map[string]Item)
	memc.evictPolicy.Reset()
	memc.unixTimeBucket.Clear()
}

func (memc *MemoryCache) Peek(key string) (Entry, bool) {
	memc.mu.RLock()
	defer memc.mu.RUnlock()

	if item, ok := memc.kv[key]; ok {
		return item, true
	}

	return Item{}, false
}

func (memc *MemoryCache) Keys() []string {
	memc.mu.RLock()
	defer memc.mu.RUnlock()

	keys := make([]string, 0, len(memc.kv))
	for key := range memc.kv {
		keys = append(keys, key)
	}
	return keys
}

func (memc *MemoryCache) Entries() []Entry {
	memc.mu.RLock()
	defer memc.mu.RUnlock()

	entries := make([]Entry, 0, len(memc.kv))
	for _, entry := range memc.kv {
		entries = append(entries, entry)
	}
	return entries
}

func (memc *MemoryCache) Size() int {
	memc.mu.RLock()
	defer memc.mu.RUnlock()
	return len(memc.kv)
}

func (memc *MemoryCache) Cap() int {
	memc.mu.RLock()
	defer memc.mu.RUnlock()
	return memc.cap
}

func (memc *MemoryCache) Resize(cap int) {
	memc.mu.Lock()
	defer memc.mu.Unlock()
	memc.cap = cap
}

func (memc *MemoryCache) Recover(entries []Entry) {
	memc.Purge()
	memc.mu.Lock()
	defer memc.mu.Unlock()
	for _, entry := range entries[:min(len(entries), memc.cap)] {
		if entry.TTL() != 0 {
			memc.kv[entry.Key()] = Item{
				key:          entry.Key(),
				value:        entry.Value(),
				lastUpdated:  entry.LastUpdated(),
				creationTime: entry.CreationTime(),
				expiryTime:   entry.ExpiryTime(),
				metadata:     entry.Metadata(),
			}
			memc.evictPolicy.Register(Set, entry.Key())
			memc.unixTimeBucket.AddKeyToBucket(entry.ExpiryTime(), entry.Key())
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
				memc.unixTimeBucket.PruneBucket(time.Now().Add(time.Second), func(k, v interface{}) bool {
					go memc.Delete(k.(string))
					return true
				})
			}
		}
	}()
}

func WithCapacity(cap int) Option {
	return func(memc *MemoryCache) {
		if cap > 0 {
			memc.cap = cap
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

func (utb *UnixTimeBucket) RemoveKeyFromBucket(timepoint time.Time, key string) {
	if expiryMap, ok := utb.timeToBucket.Load(timepoint.Unix()); ok {
		expiryMap.(*sync.Map).Delete(key)
	}
}

func (utb *UnixTimeBucket) AddKeyToBucket(timepoint time.Time, key string) {
	expiryMap, _ := utb.timeToBucket.LoadOrStore(timepoint.Unix(), new(sync.Map))
	expiryMap.(*sync.Map).Store(key, struct{}{})
}

func (utb *UnixTimeBucket) PruneBucket(timepoint time.Time, callback func(k, v interface{}) bool) {
	if expiryMap, ok := utb.timeToBucket.LoadAndDelete(timepoint.Unix()); ok {
		expiryMap.(*sync.Map).Range(callback)
	}
}

func (utb *UnixTimeBucket) Clear() {
	utb.timeToBucket.Clear()
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
