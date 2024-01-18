package box

import (
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

type Option func(*Box)

func WithDefaultTTL(t time.Duration) Option {
	return func(b *Box) {
		b.defaultTTL = t
	}
}

func New(options ...Option) *Box {
	box := &Box{
		data:       make(map[string]*Item),
		defaultTTL: -1,
	}

	for _, opt := range options {
		opt(box)
	}

	return box
}
