package cache

import (
	"container/list"
	"errors"
	"fmt"
	"math"
	"sync"
)

type EvictionPolicy interface {
	Register(Operation, string) error
	Next() (string, bool)
	Reset()
}

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

var (
	ErrPolicyKeyNotFound          = errors.New("key not found")
	ErrPolicyKeyAlreadyExists     = errors.New("key already exists")
	ErrPolicyUnsupportedOperation = errors.New("unsupported operation")
)

type LRU struct {
	mu sync.RWMutex

	keyToNode map[string]*list.Element

	list *list.List
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
			err = ErrPolicyKeyNotFound
			return
		}

		lru.list.MoveToFront(node)

	case Set:
		if _, ok := lru.keyToNode[key]; ok {
			err = ErrPolicyKeyAlreadyExists
			return
		}

		lru.keyToNode[key] = lru.list.PushFront(key)

	case Delete:
		node, ok := lru.keyToNode[key]
		if !ok {
			err = ErrPolicyKeyNotFound
			return
		}

		lru.list.Remove(node)
		delete(lru.keyToNode, key)

	default:
		err = fmt.Errorf("%w: %v", ErrPolicyUnsupportedOperation, op)
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

type LFU struct {
	mu sync.RWMutex

	keyToNode map[string]*list.Element

	freqBucket map[int]*list.List

	minFreq int
}

func NewLFUPolicy() EvictionPolicy {
	return &LFU{
		keyToNode:  make(map[string]*list.Element),
		freqBucket: make(map[int]*list.List),
	}
}

type KeyFrequencyPair struct {
	key  string
	freq int
}

func (lfu *LFU) Register(op Operation, key string) (err error) {
	lfu.mu.Lock()
	defer lfu.mu.Unlock()

	switch op {
	case Get, Update:
		node, ok := lfu.keyToNode[key]
		if !ok {
			err = ErrPolicyKeyNotFound
			return
		}

		staleFreq, _ := lfu.removeStaleKeyFromFrequencyBucket(node)
		newFreq := staleFreq + 1
		if _, ok := lfu.freqBucket[newFreq]; !ok {
			lfu.freqBucket[newFreq] = list.New()
		}

		lfu.keyToNode[key] = lfu.freqBucket[newFreq].PushFront(KeyFrequencyPair{
			key:  key,
			freq: newFreq,
		})

		if lfu.freqBucket[staleFreq].Len() == 0 && staleFreq == lfu.minFreq {
			delete(lfu.freqBucket, staleFreq)
			lfu.minFreq++
		}

	case Set:
		if _, ok := lfu.keyToNode[key]; ok {
			err = ErrPolicyKeyAlreadyExists
			return
		}

		if _, ok := lfu.freqBucket[1]; !ok {
			lfu.freqBucket[1] = list.New()
		}

		lfu.keyToNode[key] = lfu.freqBucket[1].PushFront(KeyFrequencyPair{
			key:  key,
			freq: 1,
		})
		lfu.minFreq = 1

	case Delete:
		node, ok := lfu.keyToNode[key]
		if !ok {
			err = ErrPolicyKeyNotFound
			return
		}

		delete(lfu.keyToNode, key)
		staleFreq, _ := lfu.removeStaleKeyFromFrequencyBucket(node)
		if lfu.freqBucket[staleFreq].Len() == 0 && staleFreq == lfu.minFreq {
			delete(lfu.freqBucket, staleFreq)
			lfu.updateMinFrequency()
		}

	default:
		err = fmt.Errorf("%w: %v", ErrPolicyUnsupportedOperation, op)

	}

	return err
}

func (lfu *LFU) removeStaleKeyFromFrequencyBucket(node *list.Element) (freq int, ok bool) {
	if node == nil {
		return
	}

	freq = node.Value.(KeyFrequencyPair).freq
	if lfu.freqBucket[freq] != nil {
		lfu.freqBucket[freq].Remove(node)
	}

	return freq, true
}

func (lfu *LFU) updateMinFrequency() {
	if len(lfu.freqBucket) == 0 {
		lfu.minFreq = 0
		return
	}

	min := math.MaxInt
	for freq := range lfu.freqBucket {
		if freq < min {
			min = freq
		}
	}
	lfu.minFreq = min
}

func (lfu *LFU) Next() (key string, ok bool) {
	lfu.mu.RLock()
	defer lfu.mu.RUnlock()

	if len(lfu.keyToNode) == 0 || lfu.minFreq == 0 {
		return "", false
	}

	bucket, ok := lfu.freqBucket[lfu.minFreq]
	if !ok {
		// This should never happen if minFreq is correct.
		return "", false
	}

	node := bucket.Back()
	if node == nil {
		// This should never happen if bucket exists
		return "", false
	}

	pair := node.Value.(KeyFrequencyPair)
	return pair.key, true
}

func (lfu *LFU) Reset() {
	lfu.mu.Lock()
	defer lfu.mu.Unlock()

	lfu.keyToNode = make(map[string]*list.Element)
	lfu.freqBucket = make(map[int]*list.List)
	lfu.minFreq = 0
}
