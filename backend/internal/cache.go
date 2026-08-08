package internal

import (
	"sync"
	"time"
)

type Entry struct {
	createdAt time.Time
	data      []byte
}

type Key struct {
	Car         string `json:"car_name"`
	Track       string `json:"track_name"`
	TrackLayout string `json:"layout_name"`
	AirTemp     string `json:"air_temp,omitempty"`
	TrackTemp   string `json:"track_temp,omitempty"`
	Weather     string `json:"weather,omitempty"`
}

type Cache struct {
	entries map[Key]Entry
	mut     sync.RWMutex
}

func NewCache(ttl, reapInterval time.Duration) *Cache {
	c := &Cache{entries: make(map[Key]Entry)}

	go c.reap(ttl, reapInterval)

	return c
}

func (c *Cache) Get(key Key) ([]byte, bool) {
	c.mut.RLock()
	defer c.mut.RUnlock()

	if entry, ok := c.entries[key]; ok {
		return entry.data, true
	}

	return []byte{}, false
}

func (c *Cache) Put(key Key, data []byte) {
	c.mut.Lock()
	defer c.mut.Unlock()

	c.entries[key] = Entry{
		createdAt: time.Now(),
		data:      data,
	}
}

func (c *Cache) reap(ttl, reapInterval time.Duration) {
	ticker := time.NewTicker(reapInterval)
	defer ticker.Stop()

	for range ticker.C {
		c.mut.Lock()

		for key, entry := range c.entries {
			if time.Since(entry.createdAt) > ttl {
				delete(c.entries, key)
			}
		}

		c.mut.Unlock()
	}
}
