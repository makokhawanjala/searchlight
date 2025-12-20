package searcher

import (
	"sync"

	"github.com/makokhawanjala/searchlight/internal/indexer"
)

type cacheEntry struct {
	results []*indexer.FileInfo
	prev    *cacheEntry
	next    *cacheEntry
}

type Cache struct {
	capacity int
	cache    map[string]*cacheEntry
	head     *cacheEntry
	tail     *cacheEntry
	mu       sync.RWMutex
}

func NewCache(capacity int) *Cache {
	if capacity <= 0 {
		capacity = 100
	}

	head := &cacheEntry{}
	tail := &cacheEntry{}
	head.next = tail
	tail.prev = head

	return &Cache{
		capacity: capacity,
		cache:    make(map[string]*cacheEntry),
		head:     head,
		tail:     tail,
	}
}

func (c *Cache) Get(key string) ([]*indexer.FileInfo, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, exists := c.cache[key]
	if !exists {
		return nil, false
	}

	c.moveToFront(entry)
	return entry.results, true
}

func (c *Cache) Put(key string, results []*indexer.FileInfo) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if entry, exists := c.cache[key]; exists {
		entry.results = results
		c.moveToFront(entry)
		return
	}

	entry := &cacheEntry{results: results}
	c.cache[key] = entry
	c.addToFront(entry)

	if len(c.cache) > c.capacity {
		c.removeLRU()
	}
}

func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cache = make(map[string]*cacheEntry)
	c.head.next = c.tail
	c.tail.prev = c.head
}

func (c *Cache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.cache)
}

func (c *Cache) moveToFront(entry *cacheEntry) {
	c.removeEntry(entry)
	c.addToFront(entry)
}

func (c *Cache) addToFront(entry *cacheEntry) {
	entry.next = c.head.next
	entry.prev = c.head
	c.head.next.prev = entry
	c.head.next = entry
}

func (c *Cache) removeEntry(entry *cacheEntry) {
	entry.prev.next = entry.next
	entry.next.prev = entry.prev
}

func (c *Cache) removeLRU() {
	lru := c.tail.prev
	if lru == c.head {
		return
	}

	c.removeEntry(lru)

	for key, entry := range c.cache {
		if entry == lru {
			delete(c.cache, key)
			break
		}
	}
}