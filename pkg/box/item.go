package box

import "time"

type Item struct {
	key          string
	value        interface{}
	lastUpdated  time.Time
	creationTime time.Time
	timeToLive   time.Duration
	metaData     map[string]interface{}
}
