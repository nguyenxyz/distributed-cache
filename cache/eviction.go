package cache

type EvictionPolicy interface {
	Register(Operation, string)
	Next() string
	Clear()
}

type LRU struct {
}

func (lru *LRU) Register(op Operation, key string) {

}

func (lru *LRU) Next() string {
	return "HAHA"
}

func (lru *LRU) Clear() {}
