package cache

import (
	"context"
	crand "crypto/rand"
	"math"
	"math/big"
	"reflect"
	"strconv"
	"sync"
	"testing"
	"time"
)

func TestLRUSetGet(t *testing.T) {
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

	lru := NewLRU(context.Background())
	for _, tc := range testCases {
		tc := tc
		t.Run(tc.desc, func(t *testing.T) {
			t.Parallel()

			if tc.shouldExist {
				lru.Set(tc.key, tc.value)
			}

			entry, ok := lru.Get(tc.key)
			if tc.shouldExist != ok {
				t.Errorf("Expected key existence: %t, got %t", tc.shouldExist, ok)
			}
			if !reflect.DeepEqual(entry.Value(), tc.value) {
				t.Errorf("Expected value: %v, got %v", tc.value, entry.Value())
			}

		})
	}
}

func TestLRUGarbageCollection(t *testing.T) {
	testCases := []struct {
		desc, key   string
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
			desc:        "Item with ttl of 4s should still exist after 3s",
			ttl:         4 * time.Second,
			sleep:       3 * time.Second,
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
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.desc, func(t *testing.T) {
			t.Parallel()

			lru := NewLRU(context.Background(), WithDefaultTTL(tc.ttl))
			lru.Set(tc.key, tc.key)
			time.Sleep(tc.sleep)
			if _, ok := lru.Get(tc.key); ok != tc.shouldExist {
				t.Errorf("Expected key existence: %t, got %t", tc.shouldExist, ok)
			}
		})
	}
}

func TestLRUSameKeyConcurrentWrite(t *testing.T) {
	lru := NewLRU(context.Background())
	var wg sync.WaitGroup

	numKeys := 2
	for i := 0; i < 1000; i++ {
		wg.Add(1)

		go func(key int) {
			defer wg.Done()

			k := strconv.Itoa(key % numKeys)
			lru.Set(k, k)
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
	cap := 10000
	overflowf := 50
	lru := NewLRU(context.Background(), WithCapacity(cap))
	var wg sync.WaitGroup

	for i := 0; i < cap*overflowf; i++ {
		wg.Add(1)

		go func(key int) {
			defer wg.Done()

			lru.Set(strconv.Itoa(key), key)
		}(i)
	}

	wg.Wait()

	if size := lru.Size(); size != cap {
		t.Errorf("Expected size: %d, got %d", cap, size)
	}

	t.Logf("Size: %d", lru.Size())

}

func BenchmarkLRUHitMiss_Random(b *testing.B) {
	lru := NewLRU(context.Background(), WithCapacity(8192))
	trace := make([]string, b.N)
	for i := 0; i < len(trace); i++ {
		trace[i] = randKeyFromInt64(b, 32768)
	}

	b.ResetTimer()
	var hit, miss int
	for i := 0; i < b.N; i++ {
		if i%2 == 0 {
			lru.Set(trace[i], trace[i])
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
	lru := NewLRU(context.Background(), WithCapacity(8192))
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
		lru.Set(trace[i], trace[i])
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

func randKeyFromInt64(tb testing.TB, mod int) string {
	out, err := crand.Int(crand.Reader, big.NewInt(math.MaxInt64))
	if err != nil {
		tb.Fatal(err)
	}

	return strconv.Itoa(int(out.Int64()) % mod)
}
