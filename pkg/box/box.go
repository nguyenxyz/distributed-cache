package box

import (
	"fmt"
	"log"
	"sync"
	"time"
)

type Box struct {
	// Mutex for thread-safe access to the key-value store
	mu sync.Mutex

	// The underlying key-value storage
	data map[string]*Item

	// Optional: Default time-to-live for items in the key-value store
	defaultTTL time.Duration

	// Optional: Max capacity of the key-value store
	maxCapacity int

	// Optional: Eviction strategy for managing key-value store capacity
	evictStrat EvictionStrategy

	logger log.Logger
}

func (b *Box) Get(key string) (*Item, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	item, found := b.data[key]
	if !found {
		return &Item{}, NewOperationError(fmt.Sprintf("Item with key %s doesn't exist", key), KeyNotFound)
	}

	if item.isExpired() {
		delete(b.data, key)
		return &Item{}, NewOperationError(fmt.Sprintf("Item with key %s has already expired", key), TTLExpired)
	}

	return item, nil
}

func (b *Box) Set(key string, value interface{}) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	item, found := b.data[key]
	if found {
		if !item.isExpired() {
			item.value = value
			item.lastUpdated = time.Now()
			return nil
		}
		delete(b.data, key)
	}

	if len(b.data) > b.maxCapacity && b.maxCapacity > 0 {
		evictedKey, err := b.evictStrat.Evict(b.data)
		if err != nil {
			return NewOperationError(err.Error(), Operational)
		}
		delete(b.data, evictedKey)
	}

	b.data[key] = &Item{
		key:          key,
		value:        value,
		lastUpdated:  time.Now(),
		creationTime: time.Now(),
	}

	return nil
}

type Option func(*Box)

func New(options ...Option) *Box {
	return &Box{}
}
