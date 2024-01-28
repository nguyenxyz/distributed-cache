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

func WithCapacity(cap int64) Option {
	return func(b *Box) {
		if cap > 0 {
			b.capacity = cap
		}
	}
}
