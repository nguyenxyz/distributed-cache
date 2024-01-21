package box

import (
	"context"
	"sync"
	"time"

	"github.com/ph-ngn/nanobox/pkg/util/log"
)

type Box struct {
	// Mutex for thread-safe access to the key-value store
	mu sync.RWMutex

	// The underlying key-value storage
	data map[string]*Item

	// Optional: Default time-to-live for items in the key-value store
	defaultTTL time.Duration

	// Optional: Garbage collection interval for removing expired items
	garbageCollectionInterval time.Duration

	// Channel for signaling the garbage collector to stop
	stopGarbageCollection chan struct{}

	logger log.Logger
}

func (b *Box) Get(key string) *Item {
	b.mu.RLock()
	defer b.mu.RUnlock()

	item, found := b.data[key]
	if found && item.isTTLExpired() {
		delete(b.data, key)
		return nil
	}

	return item
}

func (b *Box) Set(key string, value interface{}) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if item, found := b.data[key]; found && !item.isTTLExpired() {
		item.value = value
		item.lastUpdated = time.Now()
		return nil
	}

	b.data[key] = &Item{
		key:          key,
		value:        value,
		lastUpdated:  time.Now(),
		creationTime: time.Now(),
		setTTL:       b.defaultTTL,
	}

	return nil
}

func (b *Box) Delete(key string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	delete(b.data, key)
	return nil
}

func (b *Box) Run(ctx context.Context) {
	b.logger.Infof("Starting box service with a default TTL of %v", b.defaultTTL)
	if b.defaultTTL >= 0 {
		go b.runGarbageCollector()
		defer b.stopGarbageCollector()
	}

	defer b.logger.Infof("Shutting down box service")

	<-ctx.Done()
}

func (b *Box) runGarbageCollector() {
	b.stopGarbageCollection = make(chan struct{})
	ticker := time.NewTicker(b.garbageCollectionInterval)

	b.logger.Infof("Starting the garbage collector with interval of %v", b.garbageCollectionInterval)
	for {
		select {
		case <-ticker.C:
			b.mu.Lock()
			for k, v := range b.data {
				if v.isTTLExpired() {
					delete(b.data, k)
				}
			}
			b.mu.Unlock()

		case <-b.stopGarbageCollection:
			ticker.Stop()
			b.stopGarbageCollection = nil
			return
		}
	}
}

func (b *Box) stopGarbageCollector() {
	if b.stopGarbageCollection != nil {
		b.logger.Infof("Shutting down the garbage collector")
		b.stopGarbageCollection <- struct{}{}
	}
}

type Option func(*Box)

func WithDefaultTTL(ttl time.Duration) Option {
	return func(b *Box) {
		b.defaultTTL = ttl
	}
}

func WithGarbageCollectionInterval(interval time.Duration) Option {
	return func(b *Box) {
		b.garbageCollectionInterval = interval
	}
}

func New(options ...Option) *Box {
	box := &Box{
		data:                      make(map[string]*Item),
		defaultTTL:                -1,
		garbageCollectionInterval: 30 * time.Minute,
	}

	for _, opt := range options {
		opt(box)
	}

	return box
}
