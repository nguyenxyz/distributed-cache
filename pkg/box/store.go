package box

import "time"

// Store is the interface for a box object
type Store interface {
	// Get returns the value for the given key if found, returns error on failure
	Get(key string) Record

	// Set sets the value for the given key, returns error on failure
	Set(key string, value interface{}) error

	// Delete removes the given key if found, returns error on failure
	Delete(key string) error
}

// Record is the interface for an item object
type Record interface {
	// Key returns the key of the record
	Key() string

	// Value returns the value of the record
	Value() interface{}

	// LastUpdated returns the timestamp when the record was last updated
	LastUpdated() time.Time

	// CreationTime returns the timestramp when the recrod was first created
	CreationTime() time.Time

	// TTL returns the time-to-live duration for the record
	TTL() time.Duration

	// Metadata returns the metadata associated with the record
	Metadata() map[string]interface{}
}
