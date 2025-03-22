package cache

type EvictionPolicy interface {
	Register(Operation, string)
	Next() string
	Reset()
}

type LRU struct {
}

func (lru *LRU) Register(op Operation, key string) {

}

func (lru *LRU) Next() string {
	return "HAHA"
}

func (lru *LRU) Reset() {}

type LFU struct{}

func (lfu *LFU) Register(op Operation, key string) {}

func (lfu *LFU) Next() string { return "HAHA" }

func (lfu *LFU) Reset() {}
