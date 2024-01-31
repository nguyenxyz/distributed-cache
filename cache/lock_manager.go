package cache

import (
	"hash/maphash"
	"sync"
)

type LockManager struct {
	locks []*sync.RWMutex
	seed  maphash.Seed
}

func NewLockManager(size int) *LockManager {
	lm := &LockManager{
		locks: make([]*sync.RWMutex, size),
		seed:  maphash.MakeSeed(),
	}

	for idx := range lm.locks {
		lm.locks[idx] = new(sync.RWMutex)
	}

	return lm
}
func (lm *LockManager) Get(key string) *sync.RWMutex {
	idx := maphash.String(lm.seed, key) % uint64(len(lm.locks))
	return lm.locks[idx]
}

type AdaptiveLockManager struct {
	locks map[string]*sync.RWMutex
	mu    sync.RWMutex
}

func NewAdaptiveLockManager() *AdaptiveLockManager {
	return &AdaptiveLockManager{
		locks: make(map[string]*sync.RWMutex),
	}
}

func (alm *AdaptiveLockManager) Get(key string) *sync.RWMutex {
	alm.mu.RLock()
	if l, ok := alm.locks[key]; ok {
		alm.mu.RUnlock()
		return l
	}

	alm.mu.RUnlock()
	alm.mu.Lock()
	defer alm.mu.Unlock()
	// need to check again here before register new lock to prevent goroutines from overwriting lock for a given key when
	// they read the locks map at the same time
	if l, ok := alm.locks[key]; ok {
		return l
	}

	alm.locks[key] = new(sync.RWMutex)
	return alm.locks[key]
}

func (alm *AdaptiveLockManager) Remove(key string) {
	alm.mu.Lock()
	defer alm.mu.Unlock()

	delete(alm.locks, key)
}
