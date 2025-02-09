package cache

type EvictionPolicy interface {
	register(Operation, string)
}

type LRU struct {
}

func (lru *LRU) register(op Operation, key string) {

}
