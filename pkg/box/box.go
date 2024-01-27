package box

import (
	"container/list"
	"context"
	"sync"
	"time"

	"github.com/ph-ngn/nanobox/pkg/util/log"
)

var _ Store = (*Box)(nil)

type Box struct {
	// Mutex for thread-safe access to the key-value store
	mu sync.Mutex

	// The underlying key-value storage
	data map[string]*list.Element

	// Optional: Capacity of the key-value storage
	capacity int

	// Doubly linked list to keep track of the least recently used items
	lru list.List

	// Optional: Default time-to-live for items in the key-value store
	defaultTTL time.Duration

	// Optional: Garbage collection interval for removing expired items
	gcInterval time.Duration

	// Channel for signaling the garbage collector to stop
	gcStop chan struct{}

	logger log.Logger
}

func New(logger log.Logger, options ...Option) *Box {
	box := &Box{
		data:       make(map[string]*list.Element),
		capacity:   -1,
		defaultTTL: -1,
		gcInterval: 30 * time.Minute,
		logger:     logger,
	}

	for _, opt := range options {
		opt(box)
	}

	return box
}

func (b *Box) Run(ctx context.Context) {
	b.logger.Infof("Starting box service with a default TTL of %v", b.defaultTTL)
	if b.defaultTTL >= 0 {
		b.runGarbageCollector()
		defer b.stopGarbageCollector()
	}

	defer b.logger.Infof("Shutting down box service")

	<-ctx.Done()
}

func (b *Box) Get(key string) Record {
	b.mu.Lock()
	defer b.mu.Unlock()

	if e, ok := b.data[key]; ok {
		item := e.Value.(*Item)
		if item.isTTLExpired() {
			delete(b.data, key)
			b.lru.Remove(e)
			return nil
		}

		return item
	}

	return nil
}

func (b *Box) Set(key string, value interface{}) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if e, ok := b.data[key]; ok {
		item := e.Value.(*Item)
		if !item.isTTLExpired() {
			item.value = value
			item.lastUpdated = time.Now()
			b.lru.MoveToFront(e)
			return nil
		}

		delete(b.data, key)
		b.lru.Remove(e)
	}

	for b.capacity > 0 && len(b.data) >= b.capacity {
		back := b.lru.Back()
		delete(b.data, back.Value.(*Item).Key())
		b.lru.Remove(back)
	}

	item := &Item{
		key:          key,
		value:        value,
		lastUpdated:  time.Now(),
		creationTime: time.Now(),
		setTTL:       b.defaultTTL,
	}
	b.lru.PushFront(item)
	b.data[key] = b.lru.Front()

	return nil
}

func (b *Box) Delete(key string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if e, ok := b.data[key]; ok {
		delete(b.data, key)
		b.lru.Remove(e)
	}

	return nil
}

func (b *Box) runGarbageCollector() {
	b.gcStop = make(chan struct{})
	ticker := time.NewTicker(b.gcInterval)

	b.logger.Infof("Starting the garbage collector with interval of %v", b.gcInterval)
	go func() {
		for {
			select {
			case <-ticker.C:
				b.mu.Lock()
				for k, v := range b.data {
					if v.Value.(*Item).isTTLExpired() {
						delete(b.data, k)
						b.lru.Remove(v)
					}
				}
				b.mu.Unlock()

			case <-b.gcStop:
				ticker.Stop()
				b.gcStop = nil
				return
			}
		}
	}()
}

func (b *Box) stopGarbageCollector() {
	if b.gcStop != nil {
		b.logger.Infof("Shutting down the garbage collector")
		b.gcStop <- struct{}{}
	}
}
