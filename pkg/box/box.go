package box

import (
	"fmt"
	"log"
	"sync"
	"time"
)

type Box struct {
	// Read-Wrire mutex for thread-safe access to the key-value store
	mu sync.RWMutex

	// The underlying key-value storage
	data map[string]Item

	// Optional: Default time-to-live for items in the key-value store
	defaultTTL time.Duration

	// Optional: Max capacity of the key-value store
	maxCapacity int

	// Optional: Eviction strategy for managing key-value store capacity
	evictStrat EvictionStrategy

	logger log.Logger
}

func (b *Box) Get(key string) (Item, error) {
	b.mu.RLock()
	item, found := b.data[key]
	b.mu.RUnlock()

	if !found {
		return Item{}, NewOperationError(fmt.Sprintf("Item with key %s doesn't exist", key), KeyNotFound)
	}

	if item.timeToLive > 0 {
		elapsedTime := time.Since(item.creationTime)
		if elapsedTime > item.timeToLive {
			b.mu.Lock()
			defer b.mu.Unlock()

			delete(b.data, key)
			expiredDuration := elapsedTime - item.timeToLive
			return Item{}, NewOperationError(fmt.Sprintf("Item with key %s has expired %2.f seconds ago", key, expiredDuration.Seconds()), TTLExpired)
		}
	}

	return item, nil

}

func (b *Box) Set(key string, value interface{}) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(b.data) > b.maxCapacity && b.maxCapacity > 0 {
		evictedKey, err := b.evictStrat.Evict(b.data)
		if err != nil {
			return NewOperationError(err.Error(), Operational)
		}
		delete(b.data, evictedKey)
	}

	b.data[key] = Item{
		key:          key,
		value:        value,
		lastUpdated:  time.Now(),
		creationTime: time.Now(),
		timeToLive:   b.defaultTTL,
	}

	return nil
}

type Option func(*Box)

func New(options ...Option) *Box {
	return &Box{}
}
