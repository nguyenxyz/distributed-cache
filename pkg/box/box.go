package box

import (
	"container/list"
	"context"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ph-ngn/nanobox/pkg/util/log"
)

var _ Store = (*Box)(nil)

type Box struct {
	// The underlying key-value storage
	data map[string]*list.Element

	// Lock striping manager for concurrent access to subsets of key space in key-value storage
	dataLockManager LockManager

	// Optional: Capacity of the key-value storage
	capacity int64

	// An atomic counter that keeps track of the number of entries in the key-value storage
	size atomic.Int64

	// Lock for thread-safe access to the lru linked list
	lm sync.RWMutex

	// Doubly linked list to keep track of the least recently used entries
	lru list.List

	// Optional: Default time-to-live for entries in the key-value storage
	defaultTTL time.Duration

	// Unix time bucketed expiry map of the entries
	expiry map[int64][]*list.Element

	// Lock striping manager for for concurrent access to subsets of key space in the expiry map
	expiryLockManager LockManager

	// Channel for signaling the garbage collector to stop
	gcstop chan struct{}

	logger log.Logger
}

func New(logger log.Logger, options ...Option) *Box {
	box := &Box{
		data:       make(map[string]*list.Element),
		capacity:   -1,
		defaultTTL: -1,
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
	lock := b.dataLockManager.Get(key)
	lock.RLock()
	defer lock.RUnlock()

	if e, ok := b.data[key]; ok {
		b.lm.Lock()
		defer b.lm.Unlock()

		b.lru.MoveToFront(e)
		return e.Value.(*Item)
	}

	return nil
}

func (b *Box) Set(key string, value interface{}) error {
	lock := b.dataLockManager.Get(key)
	lock.Lock()
	defer lock.Unlock()

	if e, ok := b.data[key]; ok {
		item := e.Value.(*Item)
		item.value = value
		item.lastUpdated = time.Now()
		b.lm.Lock()
		b.lru.MoveToFront(e)
		b.lm.Unlock()

		return nil
	}

	for b.capacity > 0 && b.size.Load() >= b.capacity {
		b.lm.Lock()
		back := b.lru.Back()
		delete(b.data, back.Value.(*Item).Key())
		b.lru.Remove(back)
		b.size.Add(-1)
		b.lm.Unlock()
	}

	item := &Item{
		key:          key,
		value:        value,
		lastUpdated:  time.Now(),
		creationTime: time.Now(),
		setTTL:       b.defaultTTL,
	}

	b.lm.Lock()
	defer b.lm.Unlock()
	b.lru.PushFront(item)
	b.data[key] = b.lru.Front()
	b.size.Add(1)

	return nil
}

func (b *Box) Delete(key string) error {
	lock := b.dataLockManager.Get(key)
	lock.Lock()
	defer lock.Unlock()

	if e, ok := b.data[key]; ok {
		b.lm.Lock()
		defer b.lm.Unlock()
		delete(b.data, key)
		b.lru.Remove(e)
		b.size.Add(-1)
	}

	return nil
}

func (b *Box) Collect(ctx context.Context) (chan Record, error) {
	rc := make(chan Record)
	go func() {
		defer close(rc)

		for k, v := range b.data {
			lock := b.dataLockManager.Get(k)
			lock.RLock()

			item := v.Value.(*Item)

			select {
			case <-ctx.Done():
				lock.RUnlock()
				return

			case rc <- item:
				lock.RUnlock()
			}
		}
	}()

	return rc, nil
}

func (b *Box) runGarbageCollector() {
	b.gcstop = make(chan struct{})
	ticker := time.NewTicker(time.Second)

	go func() {
		for {
			select {
			case <-ticker.C:
				now := time.Now().Unix()
				lock := b.expiryLockManager.Get(strconv.Itoa(int(now)))
				lock.RLock()
				if l, ok := b.expiry[now]; ok {
					for _, e := range l {
						b.Delete(e.Value.(*Item).Key())
					}
				}
				lock.Unlock()

			case <-b.gcstop:
				ticker.Stop()
				b.gcstop = nil
				return
			}
		}
	}()
}

func (b *Box) stopGarbageCollector() {
	if b.gcstop != nil {
		b.logger.Infof("Shutting down the garbage collector")
		b.gcstop <- struct{}{}
	}
}
