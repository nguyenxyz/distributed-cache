package box

type EvictionStrategy interface {
	Evict(data map[string]interface{}) (evictedKey string, err error)
}
