package cache

import (
	"time"
)

type Cache interface {
	// Get returns the entry for the given key if found
	Get(key string) (Entry, bool)

	// Set sets the value for the given key, returns if an eviction occured
	Set(key string, value interface{}) bool

	// Delete removes the given key from the cache, returns if key was contained
	Delete(key string) bool

	// Purge clears all entries in the cache
	Purge()

	// Peek returns the value for the the given key if found without updating the cache's eviction policy
	Peek(key string) (Entry, bool)

	// Keys returns a slice of the keys in the cache
	Keys() []string

	// Values returns a slice of the entries in the cache
	Values() []Entry

	// Len returns the number of entries in the cache
	Len() int
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

	// TTL returns the time-to-live duration for the entry
	TTL() time.Duration

	// Metadata returns the metadata associated with the entry
	Metadata() map[string]interface{}
}
