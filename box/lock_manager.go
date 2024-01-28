package box

import "sync"

type LockManager interface {
	Get(key string) sync.RWMutex
}

type AdaptiveLockStripingManager struct {
}
