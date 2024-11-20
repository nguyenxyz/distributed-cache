package cache

type EvictionPolicy interface {
	register(Operation, string)
}
