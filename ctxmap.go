package ctxmap

import (
	"fmt"
	"maps"
	"strings"
	"sync"
	"sync/atomic"
)

// partPool recycles string slices to reduce allocations during String() reconstruction.
var partPool = sync.Pool{
	New: func() any {
		// Pre-allocate a slice with reasonable capacity to minimise growth
		s := make([]string, 0, 16)
		return &s
	},
}

// CtxMap is a thread-safe map for contextual data, optimized for repeated
// string serialization, such as in structured logging or configuration debugging.
type CtxMap struct {
	mu       sync.RWMutex
	kv       map[string]any
	prefix   string
	keySep   string
	fieldSep string
	stringer func(k string, v any) string

	prefixWithSep string              // Granular caching: key -> formatted string
	fieldCache    map[string]string   // Caches clean, formatted key-value strings
	dirtyKeys     map[string]struct{} // Set of keys needing recalculation

	// Full string caching (lock-free fast-path)
	lastStringResultVal  atomic.Value // stores string
	lastStringGeneration atomic.Uint64

	// Generation counter bumped on any mutation.
	currentGeneration atomic.Uint64
}

// Ensure CtxMap implements the fmt.Stringer interface.
var _ fmt.Stringer = (*CtxMap)(nil)

// NewCtxMap creates a new CtxMap.
//
// keySegmentSeparator is used to join prefixes (e.g., ".").
// fieldSeparator is used to join key-value pairs in the String() output (e.g., " ").
// stringer formats a single key-value pair. A default "key=value" formatter is used if nil.
func NewCtxMap(keySegmentSeparator string, fieldSeparator string, stringer func(k string, v any) string) *CtxMap {
	if stringer == nil {
		stringer = func(k string, v any) string {
			return k + "=" + fmt.Sprint(v)
		}
	}

	return &CtxMap{
		kv:         make(map[string]any),
		keySep:     keySegmentSeparator,
		fieldSep:   fieldSeparator,
		stringer:   stringer,
		fieldCache: make(map[string]string),
		dirtyKeys:  make(map[string]struct{}),
	}
}

// Get retrieves the value associated with the given raw key and a boolean indicating
// whether the key was found in the map. The prefix is not applied.
func (m *CtxMap) Get(key string) (any, bool) {
	m.mu.RLock()
	val, ok := m.kv[key]
	m.mu.RUnlock()
	return val, ok
}

// GetPrefixed retrieves the value by a key that includes the map's prefix.
// It also returns a boolean indicating whether the key was found in the map.
func (m *CtxMap) GetPrefixed(key string) (any, bool) {
	m.mu.RLock()
	prefixWithSep := m.prefixWithSep
	m.mu.RUnlock()

	if prefixWithSep == "" {
		return m.Get(key)
	}

	prefixLen := len(prefixWithSep)
	if len(key) > prefixLen && key[:prefixLen] == prefixWithSep {
		return m.Get(key[prefixLen:])
	}

	return nil, false
}

// Set sets key to value.
func (m *CtxMap) Set(key string, value any) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// We cannot safely check for equality on `value` (panic risk),
	// we must assume it's a change and invalidate
	m.kv[key] = value
	m.markDirty(key)
}

// SetMultiple sets multiple key-value pairs.
// This is more efficient than multiple Set() calls when setting many fields at once.
func (m *CtxMap) SetMultiple(pairs map[string]any) {
	if len(pairs) == 0 {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for key, value := range pairs {
		m.kv[key] = value
		m.markDirty(key)
	}
}

// Delete removes a key. The prefix is not applied.
func (m *CtxMap) Delete(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.kv[key]; exists {
		delete(m.kv, key)
		delete(m.fieldCache, key) // Remove from clean cache
		delete(m.dirtyKeys, key)  // Remove from dirty set
		m.currentGeneration.Add(1)
	}
}

// DeletePrefixed removes a key using the current prefix.
func (m *CtxMap) DeletePrefixed(key string) {
	m.mu.RLock()
	prefixWithSep := m.prefixWithSep
	m.mu.RUnlock()

	if prefixWithSep == "" {
		m.Delete(key)
		return
	}

	prefixLen := len(prefixWithSep)
	if len(key) > prefixLen && key[:prefixLen] == prefixWithSep {
		m.Delete(key[prefixLen:])
	}
}

// WithPairs returns a new map with the given alternating key-value pairs merged.
// New pairs overwrite existing ones. Incomplete pairs are skipped.
// The original map is not modified.
func (m *CtxMap) WithPairs(keyValues ...any) *CtxMap {
	if len(keyValues) < 2 {
		return m
	}

	m.mu.RLock()

	newSize := len(m.kv) + len(keyValues)/2
	kvNew := make(map[string]any, newSize)
	maps.Copy(kvNew, m.kv)

	// Copy caches
	fieldCacheNew := make(map[string]string, newSize)
	maps.Copy(fieldCacheNew, m.fieldCache)
	dirtyKeysNew := make(map[string]struct{}, len(m.dirtyKeys)+len(keyValues)/2)
	maps.Copy(dirtyKeysNew, m.dirtyKeys)

	newMap := &CtxMap{
		kv:            kvNew,
		prefix:        m.prefix,
		prefixWithSep: m.prefixWithSep,
		keySep:        m.keySep,
		fieldSep:      m.fieldSep,
		stringer:      m.stringer,
		fieldCache:    fieldCacheNew,
		dirtyKeys:     dirtyKeysNew,
	}
	m.mu.RUnlock()

	// Add new pairs and mark them as dirty
	for i := 0; i < len(keyValues)-1; i += 2 {
		key := toString(keyValues[i])
		kvNew[key] = keyValues[i+1]
		dirtyKeysNew[key] = struct{}{}
	}

	return newMap
}

// ReplaceAll replaces all key-value pairs in the map.
// Incomplete pairs are skipped.
func (m *CtxMap) ReplaceAll(keyValues ...any) {
	newSize := len(keyValues) / 2
	kvNew := make(map[string]any, newSize)

	// All keys are dirty
	dirtyKeys := make(map[string]struct{}, newSize)

	if len(keyValues) >= 2 {
		for i := 0; i < len(keyValues)-1; i += 2 {
			key := toString(keyValues[i])
			kvNew[key] = keyValues[i+1]
			dirtyKeys[key] = struct{}{}
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.kv = kvNew
	m.fieldCache = make(map[string]string, newSize)
	m.dirtyKeys = dirtyKeys
	m.currentGeneration.Add(1)
}

// Clear removes all key-value pairs from the map.
func (m *CtxMap) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.kv) > 0 {
		m.kv = make(map[string]any)
		m.fieldCache = make(map[string]string)
		m.dirtyKeys = make(map[string]struct{})
		m.currentGeneration.Add(1)
	}
}

// WithPrefix returns a new CtxMap with an added prefix.
// The original map is not modified.
func (m *CtxMap) WithPrefix(prefix string) *CtxMap {
	if prefix == "" {
		return m
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	mapLen := len(m.kv)
	kvCopy := make(map[string]any, mapLen)
	maps.Copy(kvCopy, m.kv)

	// Prefix change invalidates all formatted strings.
	// Mark all existing keys as dirty for recalculation.
	newDirtyKeys := make(map[string]struct{}, mapLen)
	for k := range kvCopy {
		newDirtyKeys[k] = struct{}{}
	}

	newPrefix := joinPrefix(m.prefix, prefix, m.keySep)

	return &CtxMap{
		kv:            kvCopy,
		prefix:        newPrefix,
		prefixWithSep: newPrefix + m.keySep,
		keySep:        m.keySep,
		fieldSep:      m.fieldSep,
		stringer:      m.stringer,
		fieldCache:    make(map[string]string, mapLen),
		dirtyKeys:     newDirtyKeys,
	}
}

// Merge returns a new map containing pairs from both maps.
// For duplicate keys, values from the 'other' map take precedence.
// All other properties of the receiver are preserved.
func (m *CtxMap) Merge(other *CtxMap) *CtxMap {
	m.mu.RLock()
	other.mu.RLock()
	defer m.mu.RUnlock()
	defer other.mu.RUnlock()

	mergedLen := len(m.kv) + len(other.kv)
	kvMerged := make(map[string]any, mergedLen)
	maps.Copy(kvMerged, m.kv)
	maps.Copy(kvMerged, other.kv)

	// Create a merged private cache (prefer other's entries on conflict)
	newFieldCache := make(map[string]string, mergedLen)
	maps.Copy(newFieldCache, m.fieldCache)

	//If prefixes and stringers match, we can inherit the other cache too.
	// Otherwise, we must invalidate keys coming from 'other'
	if m.prefixWithSep == other.prefixWithSep {
		// Safe to copy other's cache, overwriting m's cache for duplicate keys
		maps.Copy(newFieldCache, other.fieldCache)
	} else {
		// Prefixes differ: other's cache entries are invalid for this new map.
		// We must delete entries in fieldCacheNew that were overwritten by other.kv
		for k := range other.kv {
			delete(newFieldCache, k)
		}
	}

	return &CtxMap{
		kv:            kvMerged,
		prefix:        m.prefix,
		prefixWithSep: m.prefixWithSep,
		keySep:        m.keySep,
		fieldSep:      m.fieldSep,
		stringer:      m.stringer,
		fieldCache:    newFieldCache,
	}
}

// Clone returns a deep copy of the map.
func (m *CtxMap) Clone() *CtxMap {
	m.mu.RLock()
	defer m.mu.RUnlock()

	kvNew := make(map[string]any, len(m.kv))
	maps.Copy(kvNew, m.kv)

	fieldCacheNew := make(map[string]string, len(m.fieldCache))
	maps.Copy(fieldCacheNew, m.fieldCache)

	clone := &CtxMap{
		kv:            kvNew,
		prefix:        m.prefix,
		keySep:        m.keySep,
		fieldSep:      m.fieldSep,
		stringer:      m.stringer,
		prefixWithSep: m.prefixWithSep,
		fieldCache:    fieldCacheNew,
	}

	// Copy generation state to preserve the cached string if it exists
	clone.currentGeneration.Store(m.currentGeneration.Load())
	if v := m.lastStringResultVal.Load(); v != nil {
		clone.lastStringResultVal.Store(v)
		clone.lastStringGeneration.Store(m.lastStringGeneration.Load())
	}

	return clone
}

// AsMap returns the current snapshot of the map, applying the prefix to keys.
//
// IMPORTANT: For performance, this may return the internal map if no prefix is set.
// The returned map MUST NOT be modified. Use ToMapCopy() if you need a mutable copy.
func (m *CtxMap) AsMap() map[string]any {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.prefixWithSep == "" {
		// PERFORMANCE OPTIMIZATION: Return internal map directly
		// Caller MUST NOT modify this map
		return m.kv
	}

	// Must create new map when prefix is applied
	out := make(map[string]any, len(m.kv))
	for k, v := range m.kv {
		out[m.prefixWithSep+k] = v
	}

	return out
}

// ToMapCopy returns a new map containing a snapshot of the data with prefixes applied.
// The returned map is safe for modification.
func (m *CtxMap) ToMapCopy() map[string]any {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make(map[string]any, len(m.kv))

	if m.prefixWithSep == "" {
		maps.Copy(out, m.kv)
		return out
	}

	for k, v := range m.kv {
		out[m.prefixWithSep+k] = v
	}

	return out
}

// ToSliceCopy returns a new slice of alternating key-value pairs.
func (m *CtxMap) ToSliceCopy() []any {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]any, 0, len(m.kv)*2)
	prefix := m.prefixWithSep
	for k, v := range m.kv {
		out = append(out, prefix+k, v)
	}

	return out
}

// Len returns the number of key-value pairs.
func (m *CtxMap) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return len(m.kv)
}

// Range calls fn for each key/value pair of the current snapshot.
func (m *CtxMap) Range(fn func(k string, v any)) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	prefix := m.prefixWithSep
	for k, v := range m.kv {
		fn(prefix+k, v)
	}
}

// String returns a string representation of the map, optimized with caching.
// This method is the primary performance focus of the package.
func (m *CtxMap) String() string {
	// Lock-free fast path:
	if m.lastStringGeneration.Load() == m.currentGeneration.Load() {
		if v := m.lastStringResultVal.Load(); v != nil {
			return v.(string)
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check under lock
	gen := m.currentGeneration.Load()
	if m.lastStringGeneration.Load() == gen {
		if v := m.lastStringResultVal.Load(); v != nil {
			return v.(string)
		}
	}

	// If empty, trivial
	if len(m.kv) == 0 {
		m.lastStringResultVal.Store("")
		m.lastStringGeneration.Store(gen)
		return ""
	}

	// Use pooled slice to reduce allocations
	ptr := partPool.Get().(*[]string)
	parts := *ptr
	parts = parts[:0] // Reset slice, keep capacity

	prefix := m.prefixWithSep
	for k, v := range m.kv {
		// Check if key is dirty *or* not present in cache
		formatted, cached := m.fieldCache[k]
		if _, isDirty := m.dirtyKeys[k]; isDirty || !cached {
			// It was dirty or missing: recalculate and cache
			formatted = m.stringer(prefix+k, v)
			m.fieldCache[k] = formatted
		}
		// Now it's clean and cached, append it
		parts = append(parts, formatted)
	}

	// All dirty keys are now clean
	if len(m.dirtyKeys) > 0 {
		m.dirtyKeys = make(map[string]struct{})
	}

	result := strings.Join(parts, m.fieldSep)

	// Return slice to pool
	*ptr = parts
	partPool.Put(ptr)

	m.lastStringResultVal.Store(result)
	m.lastStringGeneration.Store(gen)

	return result
}

// markDirty is an internal helper to handle cache invalidation.
// This method assumes the write lock is already held.
func (m *CtxMap) markDirty(key string) {
	if m.dirtyKeys == nil {
		m.dirtyKeys = make(map[string]struct{})
	}
	m.dirtyKeys[key] = struct{}{}
	m.currentGeneration.Add(1)
}

// toString is a helper function that returns the string representation of a value.
func toString(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case int:
		return itoa64(int64(val))
	case int64:
		return itoa64(val)
	case uint:
		return utoa64(uint64(val))
	case uint64:
		return utoa64(val)
	case bool:
		if val {
			return "true"
		}
		return "false"
	default:
		return fmt.Sprint(v)
	}
}

// itoa64 converts an int64 to a string using the minimum number of bytes necessary.
// Manual buffer filling is faster than strconv for this specific use case.
// The returned string does not contain any extra leading zeros.
// If the value is negative, a single '-' character is prepended to the result.
func itoa64(val int64) string {
	if val == 0 {
		return "0"
	}

	var buf [20]byte // max int64 = 19 digits + optional sign
	i := len(buf)

	var uval uint64
	negative := val < 0
	if negative {
		// Negating math.MinInt64 would overflow, but converting to uint64 first is safe
		uval = uint64(-val)
	} else {
		uval = uint64(val)
	}

	// Build the string from right to left
	for uval > 0 {
		i--
		buf[i] = byte('0' + uval%10)
		uval /= 10
	}

	if negative {
		i--
		buf[i] = '-'
	}

	return string(buf[i:])
}

// utoa64 converts a uint64 to a string using the minimum number of bytes necessary.
// The returned string does not contain any extra leading zeros.
func utoa64(val uint64) string {
	if val == 0 {
		return "0"
	}

	var buf [20]byte // max uint64 = 20 digits
	i := len(buf)

	for val > 0 {
		i--
		buf[i] = byte('0' + val%10)
		val /= 10
	}

	return string(buf[i:])
}

// joinPrefix combines two prefixes with a separator.
func joinPrefix(existing string, new string, separator string) string {
	if existing == "" {
		return new
	}
	if new == "" {
		return existing
	}

	// Pre-allocate exact size needed
	buf := make([]byte, 0, len(existing)+len(separator)+len(new))
	buf = append(buf, existing...)
	buf = append(buf, separator...)
	buf = append(buf, new...)

	return string(buf)
}
