package cache

import "sync"

type LockManager struct {
}

func (lm *LockManager) Get(key string) sync.RWMutex {
	return sync.RWMutex{}
}

type AdaptiveLockManager struct {
}

func (alm *AdaptiveLockManager) Get(key string) sync.RWMutex {
	return sync.RWMutex{}
}
