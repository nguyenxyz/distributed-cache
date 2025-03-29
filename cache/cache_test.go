package cache

import (
	"context"
	crand "crypto/rand"
	"encoding/json"
	"math"
	"math/big"
	"math/rand"
	"reflect"
	"strconv"
	"sync"
	"testing"
	"time"
)

func TestCacheSetGet(t *testing.T) {
	testCases := []struct {
		desc, key   string
		value       interface{}
		shouldExist bool
	}{
		{
			desc:        "Set and Get existing key with string value",
			key:         "TESTKEY1",
			value:       "TESTVALUE1",
			shouldExist: true,
		},
		{
			desc:        "Set an Get existing key with int value",
			key:         "TESTKEY2",
			value:       2,
			shouldExist: true,
		},
		{
			desc:        "Set and Get existing key with slice value",
			key:         "TESTKEY3",
			value:       []interface{}{1, "2", []int{1, 2, 3}},
			shouldExist: true,
		},
		{
			desc: "Set and Get existing key with map value",
			key:  "TESTKEY4",
			value: map[string]interface{}{
				"1": 1,
				"2": "2",
				"3": []int{1, 2, 3},
				"4": map[int]string{
					1: "one",
					2: "two",
					3: "three",
					4: "cheers 🍻",
				},
			},
			shouldExist: true,
		},
		{
			desc:        "Get non-existing key",
			key:         "HAPPY NEW YEAR :)",
			value:       nil,
			shouldExist: false,
		},
	}

	lru := NewMemoryCache(context.Background())
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.desc, func(t *testing.T) {
			t.Parallel()
			var b []byte

			if tc.shouldExist {
				b, _ = json.Marshal(tc.value)
				lru.Set(tc.key, b, -1)
			}

			entry, ok := lru.Get(tc.key)
			if tc.shouldExist != ok {
				t.Errorf("Expected key existence: %t, got %t", tc.shouldExist, ok)
			}
			if !reflect.DeepEqual(entry.Value(), b) {
				if entry.Value() != nil || b != nil {
					t.Errorf("Expected value: %v, got %v", b, entry.Value())
				}
			}

		})
	}
}

func TestLRUGarbageCollection(t *testing.T) {
	testCases := []struct {
		desc        string
		ttl, sleep  time.Duration
		shouldExist bool
	}{
		{
			desc:        "Item with ttl of 2s should expire after 2s",
			ttl:         2 * time.Second,
			sleep:       2 * time.Second,
			shouldExist: false,
		},
		{
			desc:        "Item with ttl of 4s should still exist after 2s",
			ttl:         4 * time.Second,
			sleep:       2 * time.Second,
			shouldExist: true,
		},
		{
			// the garbage collection process will clean the "1s ahead" bucket
			// so at the time the item is added, it's has already started cleaning
			// the bucket the item is supposed to be in, thus the item will be neglected
			desc:        "Item with ttl of 1s should miss the collection cycle",
			ttl:         1 * time.Second,
			sleep:       2 * time.Second,
			shouldExist: true,
		},
		{
			desc:        "Item with ttl of -1 should live forever",
			ttl:         -1,
			sleep:       3 * time.Second,
			shouldExist: true,
		},
	}

	lru := NewMemoryCache(context.Background())
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.desc, func(t *testing.T) {
			t.Parallel()

			lru.Set(tc.desc, []byte("🚀"), tc.ttl)
			time.Sleep(tc.sleep)
			if _, ok := lru.Get(tc.desc); ok != tc.shouldExist {
				t.Errorf("Expected key existence: %t, got %t", tc.shouldExist, ok)
			}
		})
	}
}

func TestLRUSameKeyConcurrentWrite(t *testing.T) {
	lru := NewMemoryCache(context.Background())
	var wg sync.WaitGroup

	numKeys := 2
	for i := 0; i < 1000; i++ {
		wg.Add(1)

		go func(key int) {
			defer wg.Done()

			k := strconv.Itoa(key % numKeys)
			lru.Set(k, []byte(k), -1)
		}(i)
	}

	wg.Wait()
	if size := lru.Size(); size != numKeys {
		t.Errorf("Expected size: %d, got %d", numKeys, size)
	}

	lru.Purge()
	if size := lru.Size(); size != 0 {
		t.Errorf("Expected cache to be clear, got %d items", size)
	}
}

func TestLRUEviction(t *testing.T) {
	cap := 100
	overflowf := 4
	lru := NewMemoryCache(context.Background(), WithCapacity(cap), WithEvictionPolicy(NewLRUPolicy()))
	var wg sync.WaitGroup

	for i := 0; i < cap*overflowf; i++ {
		wg.Add(1)

		go func(key int) {
			defer wg.Done()

			k := strconv.Itoa(key)
			lru.Set(k, []byte(k), -1)
		}(i)
	}

	wg.Wait()

	if size := lru.Size(); size != cap {
		t.Errorf("Expected size: %d, got %d", cap, size)
	}

	t.Logf("Size: %d", lru.Size())
}

func BenchmarkLRUHitMiss_Random(b *testing.B) {
	lru := NewMemoryCache(context.Background(), WithCapacity(8192), WithEvictionPolicy(NewLRUPolicy()))
	trace := make([]string, b.N)
	for i := 0; i < len(trace); i++ {
		trace[i] = randKeyFromInt64(b, 32768)
	}

	b.ResetTimer()
	var hit, miss int
	for i := 0; i < b.N; i++ {
		if i%2 == 0 {
			lru.Set(trace[i], []byte(trace[i]), -1)
		} else {
			if _, ok := lru.Get(trace[i]); ok {
				hit++
			} else {
				miss++
			}
		}
	}

	b.Logf("Cache size: %d", lru.Size())
	b.Logf("hit: %d, miss: %d, ratio %f", hit, miss, float64(hit)/float64(miss))
}

func BenchmarkLRUHistMiss_Frequency(b *testing.B) {
	lru := NewMemoryCache(context.Background(), WithCapacity(8192), WithEvictionPolicy(NewLRUPolicy()))
	trace := make([]string, b.N)
	for i := 0; i < len(trace); i++ {
		if i%2 == 0 {
			trace[i] = randKeyFromInt64(b, 16384)
		} else {
			trace[i] = randKeyFromInt64(b, 32768)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lru.Set(trace[i], []byte(trace[i]), -1)
	}

	var hit, miss int
	for i := 0; i < b.N; i++ {
		if _, ok := lru.Get(trace[i]); ok {
			hit++
		} else {
			miss++
		}
	}

	b.Logf("Cache size: %d", lru.Size())
	b.Logf("hit: %d, miss: %d, ratio %f", hit, miss, float64(hit)/float64(miss))
}

func TestLFUEviction(t *testing.T) {
	cap := 100
	lfu := NewMemoryCache(context.Background(), WithCapacity(cap), WithEvictionPolicy(NewLFUPolicy()))

	// Fill the cache to capacity
	for i := 0; i < cap; i++ {
		k := strconv.Itoa(i)
		lfu.Set(k, []byte(k), -1)
	}

	// Access some keys multiple times to increase their frequency
	// Keys 0-24 accessed once (already from initialization)
	// Keys 25-49 accessed twice
	// Keys 50-74 accessed three times
	// Keys 75-99 accessed four times
	for i := 25; i < cap; i++ {
		k := strconv.Itoa(i)
		lfu.Get(k)
	}

	for i := 50; i < cap; i++ {
		k := strconv.Itoa(i)
		lfu.Get(k)
	}

	for i := 75; i < cap; i++ {
		k := strconv.Itoa(i)
		lfu.Get(k)
	}

	// Now add new items that should cause eviction
	for i := cap; i < cap+25; i++ {
		k := strconv.Itoa(i)
		lfu.Set(k, []byte(k), -1)
	}

	// Check that the cache is still at capacity
	if size := lfu.Size(); size != cap {
		t.Errorf("Expected size: %d, got %d", cap, size)
	}

	// The least frequently used items (0-24) should be evicted first
	for i := 0; i < 25; i++ {
		k := strconv.Itoa(i)
		if _, ok := lfu.Get(k); ok {
			t.Errorf("Expected key %s to be evicted, but it still exists", k)
		}
	}

	// Keys with higher frequency (25 and above) should still exist
	for i := 25; i < cap; i++ {
		k := strconv.Itoa(i)
		if _, ok := lfu.Get(k); !ok {
			t.Errorf("Expected key %s to exist, but it was evicted", k)
		}
	}

	// New items (100-124) should exist
	for i := cap; i < cap+25; i++ {
		k := strconv.Itoa(i)
		if _, ok := lfu.Get(k); !ok {
			t.Errorf("Expected key %s to exist, but it doesn't", k)
		}
	}

	t.Logf("Size: %d", lfu.Size())
}

func BenchmarkLFUHitMiss_Random(b *testing.B) {
	lfu := NewMemoryCache(context.Background(), WithCapacity(8192), WithEvictionPolicy(NewLFUPolicy()))
	trace := make([]string, b.N)
	for i := 0; i < len(trace); i++ {
		trace[i] = randKeyFromInt64(b, 32768)
	}

	b.ResetTimer()
	var hit, miss int
	for i := 0; i < b.N; i++ {
		if i%2 == 0 {
			lfu.Set(trace[i], []byte(trace[i]), -1)
		} else {
			if _, ok := lfu.Get(trace[i]); ok {
				hit++
			} else {
				miss++
			}
		}
	}

	b.Logf("Cache size: %d", lfu.Size())
	b.Logf("hit: %d, miss: %d, ratio %f", hit, miss, float64(hit)/float64(miss))
}

func BenchmarkLFUHitMiss_Zipfian(b *testing.B) {
	lfu := NewMemoryCache(context.Background(), WithCapacity(8192), WithEvictionPolicy(NewLFUPolicy()))

	// Generate a Zipfian distribution which better represents real-world cache access patterns
	// A few items are accessed very frequently, while most items are accessed rarely
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	zipf := rand.NewZipf(r, 1.5, 1.0, 32768-1) // s=1.5 gives a moderate skew

	trace := make([]string, b.N)
	for i := 0; i < len(trace); i++ {
		trace[i] = strconv.Itoa(int(zipf.Uint64()))
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		lfu.Set(trace[i], []byte(trace[i]), -1)
	}

	var hit, miss int
	for i := 0; i < b.N; i++ {
		if _, ok := lfu.Get(trace[i]); ok {
			hit++
		} else {
			miss++
		}
	}

	b.Logf("Cache size: %d", lfu.Size())
	b.Logf("hit: %d, miss: %d, ratio %f", hit, miss, float64(hit)/float64(miss))
}

func randKeyFromInt64(tb testing.TB, mod int) string {
	out, err := crand.Int(crand.Reader, big.NewInt(math.MaxInt64))
	if err != nil {
		tb.Fatal(err)
	}

	return strconv.Itoa(int(out.Int64()) % mod)
}
