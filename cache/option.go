package cache

import (
	"time"
)

type Option func(*LRUCache)

func WithDefaultTTL(ttl time.Duration) Option {
	return func(lc *LRUCache) {
		if ttl > 0 {
			lc.ttl = ttl
		}
	}
}

func WithCapacity(cap int) Option {
	return func(lc *LRUCache) {
		if cap > 0 {
			lc.cap = cap
		}
	}
}
