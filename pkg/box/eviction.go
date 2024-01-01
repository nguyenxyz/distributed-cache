package box

type EvictionStrategy interface {
	Evict(data map[string]Item) (evictedKey string, err error)
}
