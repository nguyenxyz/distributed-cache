package cache

import (
	"time"
)

type Cache interface {
	// Get returns the entry for the given key if found
	Get(key string) (Entry, bool)

	// Set sets the value for the given key, overwrite if key exists, returns if key is overwritten
	Set(key string, value interface{}) bool

	// Update updates the value for the given key without resetting ttl, returns if key exists
	Update(key string, value interface{}) bool

	// Delete removes the given key from the cache, returns if key was contained
	Delete(key string) bool

	// Purge removes all keys currently in the cache
	Purge()

	// Peek returns the entry for the the given key if found without updating the cache's eviction policy
	Peek(key string) (Entry, bool)

	// Keys returns a slice of the keys in the cache
	Keys() []string

	// Values returns a slice of the entries in the cache
	Entries() []Entry

	// Len returns the number of entries in the cache
	Len() int

	// Cap returns the current capacity of the cache
	Cap() int

	// Resize resizes the cache with the provided capacity, overflowing entries will be evicted
	Resize(cap int)

	// UpdateDefaultTTL updates the default time-to-live of cache entries to the given duration
	UpdateDefaultTTL(ttl time.Duration)

	// DefaultTTL returns the default time-to-live of cache entries
	DefaultTTL() time.Duration
}

type Entry interface {
	// Key returns the key associated with the entry
	Key() string

	// Value returns the value associated with the entry
	Value() interface{}

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
