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
	EvictionCallBack func(key string, value []byte)

	// Option is used to apply configurations to the cache
	Option func(*MemoryCache)

	// Unix time bucket is used by garbage collector to manage expirable keys
	UnixTimeBucket struct {
		m map[int]map[string]struct{}
	}

	// Operation is used to register an operation with the eviction policy
	Operation int16
)

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
	kv map[string]Item

	// Mutex for concurrent access
	kvm sync.RWMutex

	// Optional: Capacity of the cache
	cap atomic.Int64

	// Optional: Eviction policy for the cache
	evictPolicy EvictionPolicy

	// Unix time bucket for expirable keys in the cache
	utb UnixTimeBucket

	// Callback when a cache entry is evicted
	onEvict EvictionCallBack
}

func NewMemoryCache(ctx context.Context, options ...Option) *MemoryCache {
	cache := &MemoryCache{
		kv: make(map[string]Item),
	}
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

}

func (mc *MemoryCache) Update(key string, value []byte) bool {

}

func (mc *MemoryCache) Delete(key string) bool {

}

func (mc *MemoryCache) Purge() {

}

func (mc *MemoryCache) Peek(key string) (Entry, bool) {

}

func (mc *MemoryCache) Keys() []string {

}

func (mc *MemoryCache) Entries() []Entry {

}

func (mc *MemoryCache) Size() int64 {

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

}

func (mc *MemoryCache) runGarbageCollection(ctx context.Context) {

}

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
