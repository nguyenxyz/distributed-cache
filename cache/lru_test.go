package cache

import (
	"context"
	"reflect"
	"testing"
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
	for idx, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			if idx < len(testCases)-1 {
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
