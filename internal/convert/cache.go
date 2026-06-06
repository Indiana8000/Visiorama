package convert

import (
	"sync"
	"time"
)

type entry struct {
	data      []byte
	expiresAt time.Time
}

// Cache is an in-memory store for converted image bytes with TTL eviction.
type Cache struct {
	mu      sync.Mutex
	items   map[int64]*entry
	inflight map[int64]*call
}

type call struct {
	wg  sync.WaitGroup
	val []byte
	err error
}

func NewCache() *Cache {
	c := &Cache{
		items:    make(map[int64]*entry),
		inflight: make(map[int64]*call),
	}
	go c.evictLoop()
	return c
}

// Do returns cached bytes or calls fn once (singleflight) and caches the result for ttl.
func (c *Cache) Do(id int64, ttl time.Duration, fn func() ([]byte, error)) ([]byte, error) {
	c.mu.Lock()
	if e, ok := c.items[id]; ok && time.Now().Before(e.expiresAt) {
		c.mu.Unlock()
		return e.data, nil
	}
	if cl, ok := c.inflight[id]; ok {
		c.mu.Unlock()
		cl.wg.Wait()
		return cl.val, cl.err
	}
	cl := &call{}
	cl.wg.Add(1)
	c.inflight[id] = cl
	c.mu.Unlock()

	cl.val, cl.err = fn()
	cl.wg.Done()

	c.mu.Lock()
	delete(c.inflight, id)
	if cl.err == nil {
		c.items[id] = &entry{data: cl.val, expiresAt: time.Now().Add(ttl)}
	}
	c.mu.Unlock()

	return cl.val, cl.err
}

func (c *Cache) evictLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		now := time.Now()
		c.mu.Lock()
		for id, e := range c.items {
			if now.After(e.expiresAt) {
				delete(c.items, id)
			}
		}
		c.mu.Unlock()
	}
}
