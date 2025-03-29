package cache

import (
	"container/list"
	"errors"
	"fmt"
	"sync"
)

var (
	ErrKeyNotFound          = errors.New("key not found for Get/Update/Delete operation")
	ErrKeyAlreadyExists     = errors.New("key already exists and can not be re-registered with Set operation")
	ErrUnsupportedOperation = errors.New("unsupported operation")
)

// Operation is used to register an operation with the eviction policy
type Operation int16

const (
	Get Operation = iota
	Set
	Update
	Delete
)

func (op Operation) String() string {
	switch op {
	case Get:
		return "Get"

	case Set:
		return "Set"

	case Update:
		return "Update"

	case Delete:
		return "Delete"

	default:
		return fmt.Sprintf("UnknownOperation(%d)", op)
	}
}

type EvictionPolicy interface {
	Register(Operation, string) error
	Next() (string, bool)
	Reset()
}

type LRU struct {
	keyToNode map[string]*list.Element

	list *list.List

	mu sync.RWMutex
}

func NewLRUPolicy() EvictionPolicy {
	return &LRU{
		keyToNode: make(map[string]*list.Element),
		list:      list.New(),
	}
}

func (lru *LRU) Register(op Operation, key string) (err error) {
	lru.mu.Lock()
	defer lru.mu.Unlock()

	switch op {
	case Get, Update:
		node, ok := lru.keyToNode[key]
		if !ok {
			err = ErrKeyNotFound
			return
		}

		lru.list.MoveToFront(node)

	case Set:
		if _, ok := lru.keyToNode[key]; ok {
			err = ErrKeyAlreadyExists
			return
		}

		lru.keyToNode[key] = lru.list.PushFront(key)

	case Delete:
		node, ok := lru.keyToNode[key]
		if !ok {
			err = ErrKeyNotFound
			return
		}

		lru.list.Remove(node)
		delete(lru.keyToNode, key)

	default:
		err = fmt.Errorf("%w: %v", ErrUnsupportedOperation, op)
	}

	return err
}

func (lru *LRU) Next() (key string, ok bool) {
	lru.mu.RLock()
	defer lru.mu.RUnlock()

	if node := lru.list.Back(); node != nil {
		return node.Value.(string), true
	}

	return "", false
}

func (lru *LRU) Reset() {
	lru.mu.Lock()
	defer lru.mu.Unlock()

	lru.keyToNode = make(map[string]*list.Element)
	lru.list.Init()
}

type LFU struct{}

func (lfu *LFU) Register(op Operation, key string) {}

func (lfu *LFU) Next() string { return "HAHA" }

func (lfu *LFU) Reset() {}
