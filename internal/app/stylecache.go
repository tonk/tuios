package app

import (
	"hash/maphash"
	"image/color"
	"sync"
	"sync/atomic"

	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

// styleEntry holds a cached style together with its derived ANSI escape prefix
// and suffix. The escape is what the render loop writes; it is built from the
// cell's uv.Style, which carries attributes lipgloss has no setter for
// (conceal, rapid blink), so style is the closest lipgloss rendering and the
// prefix is authoritative.
type styleEntry struct {
	style  lipgloss.Style
	prefix string
	suffix string
}

// StyleCache provides thread-safe caching of lipgloss styles with automatic eviction.
// It significantly reduces allocation pressure by reusing style objects for identical cell attributes.
type StyleCache struct {
	mu    sync.RWMutex
	cache map[uint64]styleEntry
	seed  maphash.Seed

	// Statistics for monitoring (atomic counters)
	hits   atomic.Uint64
	misses atomic.Uint64
	evicts atomic.Uint64

	maxSize int // Maximum cache entries before eviction
}

// NewStyleCache creates a new style cache with the specified maximum size.
// Recommended size: 256-1024 entries (covers most terminal use cases).
func NewStyleCache(maxSize int) *StyleCache {
	if maxSize <= 0 {
		maxSize = 512 // Default size
	}
	return &StyleCache{
		cache:   make(map[uint64]styleEntry, maxSize),
		seed:    maphash.MakeSeed(),
		maxSize: maxSize,
	}
}

// hashCellAttrs creates a hash key from cell attributes.
// The hash covers everything that changes the emitted escape: the text
// attributes, the underline style, the three colours tagged by kind (basic,
// indexed, RGB), and the cursor state. Hashing colours by RGBA alone put
// ansi.BasicColor(1) and #800000 in one entry, so whichever was cached first
// was drawn for both; leaving the underline out did the same to a plain and a
// curly underline.
func (sc *StyleCache) hashCellAttrs(cell *uv.Cell, isCursor bool, isOptimized bool) uint64 {
	var h maphash.Hash
	h.SetSeed(sc.seed)

	// Hash cursor state (1 bit)
	if isCursor {
		_ = h.WriteByte(1)
	} else {
		_ = h.WriteByte(0)
	}

	// Hash optimized flag (1 bit)
	if isOptimized {
		_ = h.WriteByte(1)
	} else {
		_ = h.WriteByte(0)
	}

	if cell == nil {
		_ = h.WriteByte(0)
		return h.Sum64()
	}

	_ = h.WriteByte(1)

	_ = h.WriteByte(cell.Style.Attrs)
	_ = h.WriteByte(byte(cell.Style.Underline))
	hashColor(&h, cell.Style.Fg)
	hashColor(&h, cell.Style.Bg)
	hashColor(&h, cell.Style.UnderlineColor)

	return h.Sum64()
}

// hashColor writes c's kind and value into h.
func hashColor(h *maphash.Hash, c color.Color) {
	kind, index := classifyColor(c)
	_ = h.WriteByte(kind)
	switch kind {
	case colorKindBasic, colorKindIndexed:
		_ = h.WriteByte(index)
	case colorKindRGB:
		if !isColorSafe(c) {
			_ = h.WriteByte(0xff)
			return
		}
		r, g, b, a := c.RGBA()
		for _, v := range [4]uint32{r, g, b, a} {
			_ = h.WriteByte(byte(v >> 8))
			_ = h.WriteByte(byte(v))
		}
	}
}

// getEntry retrieves a cached style entry or builds and caches it if not found.
//
// The cell's colours are taken as they are: the render loop resolves the
// theme before it gets here, so the key is the colour actually drawn. The
// optimized flag is part of the key only; both modes carry every attribute,
// because the unfocused copy-mode render that asks for it showed reverse,
// bold and italic text as plain otherwise.
func (sc *StyleCache) getEntry(cell *uv.Cell, isCursor bool, optimized bool) styleEntry {
	hash := sc.hashCellAttrs(cell, isCursor, optimized)

	// Fast path: try read lock first
	sc.mu.RLock()
	if entry, ok := sc.cache[hash]; ok {
		sc.mu.RUnlock()
		sc.hits.Add(1)
		return entry
	}
	sc.mu.RUnlock()

	// Cache miss: build style and cache it
	sc.misses.Add(1)

	display := cellDisplayStyle(cell, isCursor)
	entry := styleEntry{style: buildCellStyle(cell, isCursor)}
	if entry.prefix = cellSGR(&display); entry.prefix != "" {
		entry.suffix = "\x1b[0m"
	}

	// Store in cache with write lock
	sc.mu.Lock()
	defer sc.mu.Unlock()

	// Check size and evict if necessary (simple LRU approximation: clear half the cache)
	if len(sc.cache) >= sc.maxSize {
		sc.evictHalf()
	}

	sc.cache[hash] = entry
	return entry
}

// Get retrieves a cached style or builds and caches it if not found.
// This is the main entry point for cached style access.
func (sc *StyleCache) Get(cell *uv.Cell, isCursor bool, optimized bool) lipgloss.Style {
	return sc.getEntry(cell, isCursor, optimized).style
}

// GetWithANSI retrieves a cached style together with its derived ANSI escape
// prefix and suffix, building and caching the entry on a miss. The escape is a
// pure function of the style, so the render loop can emit the cached prefix and
// suffix directly instead of re-deriving them via styleToANSI on every flush.
func (sc *StyleCache) GetWithANSI(cell *uv.Cell, isCursor bool, optimized bool) (lipgloss.Style, string, string) {
	entry := sc.getEntry(cell, isCursor, optimized)
	return entry.style, entry.prefix, entry.suffix
}

// evictHalf removes approximately half of the cache entries.
// This is a simple but effective eviction strategy that maintains good hit rates
// while preventing unbounded growth. Must be called with write lock held.
func (sc *StyleCache) evictHalf() {
	targetSize := sc.maxSize / 2
	evicted := 0

	// Delete entries until we reach target size
	// Note: map iteration order is randomized in Go, providing natural LRU-like behavior
	for key := range sc.cache {
		delete(sc.cache, key)
		evicted++
		if len(sc.cache) <= targetSize {
			break
		}
	}

	if evicted > 0 {
		sc.evicts.Add(uint64(evicted))
	}
}

// Clear removes all entries from the cache.
func (sc *StyleCache) Clear() {
	sc.mu.Lock()
	defer sc.mu.Unlock()

	cleared := len(sc.cache)
	// Create new map instead of deleting entries (faster)
	sc.cache = make(map[uint64]styleEntry, sc.maxSize)
	sc.evicts.Add(uint64(cleared))
}

// StyleCacheStats holds cache statistics for monitoring and debugging.
type StyleCacheStats struct {
	Hits     uint64  // Number of cache hits
	Misses   uint64  // Number of cache misses
	Evicts   uint64  // Number of evicted entries
	Size     int     // Current cache size
	HitRate  float64 // Hit rate percentage (0-100)
	Capacity int     // Maximum cache capacity
}

// GetStats returns current cache statistics.
func (sc *StyleCache) GetStats() StyleCacheStats {
	sc.mu.RLock()
	size := len(sc.cache)
	sc.mu.RUnlock()

	hits := sc.hits.Load()
	misses := sc.misses.Load()
	evicts := sc.evicts.Load()

	total := hits + misses
	hitRate := 0.0
	if total > 0 {
		hitRate = float64(hits) / float64(total) * 100.0
	}

	return StyleCacheStats{
		Hits:     hits,
		Misses:   misses,
		Evicts:   evicts,
		Size:     size,
		HitRate:  hitRate,
		Capacity: sc.maxSize,
	}
}

// ResetStats resets all statistics counters to zero.
func (sc *StyleCache) ResetStats() {
	sc.hits.Store(0)
	sc.misses.Store(0)
	sc.evicts.Store(0)
}

// Global style cache instance
var globalStyleCache = NewStyleCache(1024)

// GetGlobalStyleCache returns the global style cache instance.
// This is used by the rendering functions to cache styles across all windows.
func GetGlobalStyleCache() *StyleCache {
	return globalStyleCache
}
