package box

import "time"

type Item struct {
	key          string
	value        interface{}
	lastUpdated  time.Time
	creationTime time.Time
	setTTL       time.Duration
	metaData     map[string]interface{}
}

func (i *Item) Key() string {
	return i.key
}

func (i *Item) Value() interface{} {
	return i.value
}

func (i *Item) LastUpdated() time.Time {
	return i.lastUpdated
}

func (i *Item) CreationTime() time.Time {
	return i.creationTime
}

func (i *Item) TTL() time.Duration {
	elapsedTime := time.Since(i.creationTime)
	remainingTime := i.setTTL - elapsedTime
	if remainingTime < 0 {
		remainingTime = 0
	}

	return remainingTime
}

func (i *Item) Metadata() map[string]interface{} {
	return i.metaData
}

func (i *Item) isExpired() bool {
	return i.TTL() == 0
}
