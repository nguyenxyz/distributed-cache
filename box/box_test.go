package box

import (
	"reflect"
	"testing"
	"time"

	"github.com/ph-ngn/nanobox/util/log"
)

func TestBoxGet(t *testing.T) {
	lastUpdated := time.Now()
	creationTime := time.Now()
	mockData := map[string]*Item{
		"testKey1": {
			key:          "testKey1",
			value:        "testValue1",
			lastUpdated:  lastUpdated,
			creationTime: creationTime,
			setTTL:       -1,
		},
		"testKey2": {
			key:          "testKey2",
			value:        2,
			lastUpdated:  lastUpdated,
			creationTime: creationTime,
			setTTL:       -1,
		},
		"testKey3": {
			key:          "testKey3",
			value:        []interface{}{1, 2, "3"},
			lastUpdated:  lastUpdated,
			creationTime: creationTime,
			setTTL:       -1,
		},
		"testKey4": {
			key:          "testKey4",
			value:        map[string]interface{}{"1": 1, "2": 2, "3": "three"},
			lastUpdated:  lastUpdated,
			creationTime: creationTime,
			setTTL:       -1,
		},
	}

	testCases := []struct {
		desc          string
		key           string
		expectedValue interface{}
	}{
		{
			desc:          "Get existing key with string value",
			key:           "testKey1",
			expectedValue: "testValue1",
		},
		{
			desc:          "Get existing key with int value",
			key:           "testKey2",
			expectedValue: 2,
		},
		{
			desc:          "Get existing key with slice value",
			key:           "testKey3",
			expectedValue: []interface{}{1, 2, "3"},
		},
		{
			desc:          "Get existing key with map value",
			key:           "testKey4",
			expectedValue: map[string]interface{}{"1": 1, "2": 2, "3": "three"},
		},
		{
			desc:          "Get non-existing key",
			key:           "nonExistingKey",
			expectedValue: nil,
		},
	}

	box := New(log.NewZapLogger(log.Config{}))
	box.kv = mockData

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			item := box.Get(tc.key)
			if tc.expectedValue == nil && item != nil {
				t.Fatalf("Expected nil, got: %v", item.Value())
			}

			if tc.expectedValue != nil {
				if item == nil {
					t.Fatalf("Expected %v, got: nil", tc.expectedValue)
				}

				if !reflect.DeepEqual(item.Value(), tc.expectedValue) {
					t.Fatalf("Expected %v, got: %v", tc.expectedValue, item.Value())
				}
			}
		})
	}
}

func TestBoxSet(t *testing.T) {

}
