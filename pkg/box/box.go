package box

import (
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

func (b *Box) Get(key string) (Item, OperationError) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return Item{}, OperationError{}
}

type Option func(*Box)

func New(options ...Option) *Box {
	return &Box{}
}
