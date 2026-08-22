package internal

import (
	"math"
	"sync"
	"time"
)

// TempsThreshold
// The threshold to give cached setups within this range (in ºC)
const TempsThreshold = 3

// Entry
// Cache entry, where the setup data is stored
type Entry struct {
	createdAt  time.Time
	data       []byte
	comparison ShadyComparison
}

// Key
// Cache key, to identify the setups based on the car and track configurations
type Key struct {
	Car         string `json:"car_name"`
	Track       string `json:"track_name"`
	TrackLayout string `json:"layout_name"`
}

// Cache
// Full cache structure for the app
type Cache struct {
	entries map[Key]Entry
	mut     sync.RWMutex
}

// ShadyComparison
// If the cached setup has the same weather and close temps,
// It is not like the LLM has got it right for the original,
// so it is better to just give the cached for similar temps
type ShadyComparison struct {
	AirTemp   *float64 `json:"air_temp"`
	TrackTemp *float64 `json:"track_temp"`
	Weather   string   `json:"weather"`
}

// NewCache
// Creates and returns a new empty cache
// Takes the time to live (ttl) of the entries, and the interval between cache reaps (clean old entries)
func NewCache(ttl, reapInterval time.Duration) *Cache {
	c := &Cache{entries: make(map[Key]Entry)}

	go c.reap(ttl, reapInterval)

	return c
}

// Put
// Add a new entry to the cache
func (c *Cache) Put(key Key, comparison ShadyComparison, data []byte) {
	c.mut.Lock()
	defer c.mut.Unlock()

	c.entries[key] = Entry{
		createdAt:  time.Now(),
		data:       data,
		comparison: comparison,
	}
}

// Clean old entries based on the ttl
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

// GetCacheSetup
// Exposed function (the one to be used)
// Returns a cached setup, if the key exists and the temps are within the range (or are nil)
func (c *Cache) GetCacheSetup(key Key, comparison ShadyComparison) []byte {
	c.mut.RLock()
	cachedSetup, hit := c.entries[key]
	c.mut.RUnlock()
	if !hit {
		return nil
	}

	// Check the weather first, if one is for dry and another for wet, do not bother
	if comparison.Weather != "" && comparison.Weather != cachedSetup.comparison.Weather {
		return nil
	}

	if *comparison.TrackTemp == -1 || math.Abs(*comparison.TrackTemp-*cachedSetup.comparison.TrackTemp) <= TempsThreshold {
		if *comparison.AirTemp == -1 || math.Abs(*comparison.AirTemp-*cachedSetup.comparison.AirTemp) <= TempsThreshold {
			return cachedSetup.data
		}
	}

	return nil
}
