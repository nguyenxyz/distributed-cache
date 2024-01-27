package box

import (
	"time"
)

type Option func(*Box)

func WithDefaultTTL(ttl time.Duration) Option {
	return func(b *Box) {
		if ttl > 0 {
			b.defaultTTL = ttl
		}
	}
}

func WithGarbageCollectionInterval(interval time.Duration) Option {
	return func(b *Box) {
		if interval > 0 {
			b.gcInterval = interval
		}
	}
}

func WithCapacity(cap int) Option {
	return func(b *Box) {
		if cap > 0 {
			b.capacity = cap
		}
	}
}
