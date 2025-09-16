package ctxmap

import (
	"fmt"
	"maps"
	"strings"
	"sync"
)

// CtxMap is a thread-safe map for contextual data, optimized for repeated
// string serialization, such as in structured logging or configuration debugging.
//
// It uses a sync.RWMutex for efficient concurrent reads and a caching system
// to minimize the cost of converting the map to a string.
type CtxMap struct {
	mu       sync.RWMutex
	kv       map[string]any
	prefix   string
	keySep   string
	fieldSep string
	stringer func(k string, v any) string

	// Granular caching: track each field independently
	prefixWithSep string              // cached prefix + separator
	fieldCache    map[string]string   // key -> formatted string cache
	dirtyKeys     map[string]struct{} // Set of keys needing recalculation

	// Full string caching for when no fields changed
	lastStringResult     string
	lastStringGeneration uint64
	currentGeneration    uint64
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
		kv:       make(map[string]any),
		keySep:   keySegmentSeparator,
		fieldSep: fieldSeparator,
		stringer: stringer,

		// Caching fields initialized on demand or when needed
		fieldCache: map[string]string{},
		dirtyKeys:  map[string]struct{}{},
	}
}

// Get retrieves the value associated with the given raw key and a boolean indicating
// whether the key was found in the map. The prefix is not applied.
func (m *CtxMap) Get(key string) (any, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	v, ok := m.kv[key]
	return v, ok
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

// Set sets key to value with granular cache invalidation.
func (m *CtxMap) Set(key string, value any) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if this is actually a change to avoid unnecessary invalidation
	if oldVal, exists := m.kv[key]; exists && oldVal == value {
		return // No change, no cache invalidation needed
	}

	m.kv[key] = value
	m.markDirty(key)
}

// SetMultiple sets multiple key-value pairs efficiently with minimal cache invalidation.
// This is more efficient than multiple Set() calls when setting many fields at once.
func (m *CtxMap) SetMultiple(pairs map[string]any) {
	if len(pairs) == 0 {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for key, value := range pairs {
		// Check if this is actually a change
		if oldVal, exists := m.kv[key]; !exists || oldVal != value {
			m.kv[key] = value
			m.markDirty(key)
		}
	}
}

// Delete removes a key. The prefix is not applied.
func (m *CtxMap) Delete(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.kv[key]; exists {
		delete(m.kv, key)
		delete(m.fieldCache, key)
		m.markDirty(key)
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

	// Copy configuration
	newMap := &CtxMap{
		kv:            kvNew,
		prefix:        m.prefix,
		prefixWithSep: m.prefixWithSep,
		keySep:        m.keySep,
		fieldSep:      m.fieldSep,
		stringer:      m.stringer,
		fieldCache:    make(map[string]string, newSize),
		dirtyKeys:     map[string]struct{}{},
	}

	// Copy all field cache entries
	maps.Copy(newMap.fieldCache, m.fieldCache)

	m.mu.RUnlock()

	// Add new pairs
	for i := 0; i < len(keyValues)-1; i += 2 {
		kvNew[toString(keyValues[i])] = keyValues[i+1]
	}

	return newMap
}

// ReplaceAll replaces all key-value pairs in the map.
// Incomplete pairs are skipped.
func (m *CtxMap) ReplaceAll(keyValues ...any) {
	newSize := len(keyValues) / 2
	kvNew := make(map[string]any, newSize)

	if len(keyValues) >= 2 {
		for i := 0; i < len(keyValues)-1; i += 2 {
			kvNew[toString(keyValues[i])] = keyValues[i+1]
		}
	}

	// All keys are dirty
	dirtyKeys := map[string]struct{}{}
	for k := range kvNew {
		dirtyKeys[k] = struct{}{}
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.kv = kvNew
	m.fieldCache = make(map[string]string, newSize)
	m.dirtyKeys = dirtyKeys
	m.currentGeneration++
}

// Clear removes all key-value pairs from the map.
func (m *CtxMap) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.kv) > 0 {
		m.kv = make(map[string]any)
		m.fieldCache = map[string]string{}
		m.dirtyKeys = map[string]struct{}{}
		m.currentGeneration++
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

	newPrefix := joinPrefix(m.prefix, prefix, m.keySep)

	newMap := &CtxMap{
		kv:            kvCopy,
		prefix:        newPrefix,
		prefixWithSep: newPrefix + m.keySep,
		keySep:        m.keySep,
		fieldSep:      m.fieldSep,
		stringer:      m.stringer,
		fieldCache:    make(map[string]string, mapLen),
		dirtyKeys:     map[string]struct{}{},
	}

	return newMap
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

	newMap := &CtxMap{
		kv:            kvMerged,
		prefix:        m.prefix,
		prefixWithSep: m.prefixWithSep,
		keySep:        m.keySep,
		fieldSep:      m.fieldSep,
		stringer:      m.stringer,
		fieldCache:    make(map[string]string, mergedLen),
		dirtyKeys:     map[string]struct{}{},
	}

	// Copy the caches, prioritizing 'other' for conflicts
	maps.Copy(newMap.fieldCache, m.fieldCache)
	maps.Copy(newMap.fieldCache, other.fieldCache)

	return newMap
}

// Clone returns a deep copy of the map.
func (m *CtxMap) Clone() *CtxMap {
	m.mu.RLock()
	defer m.mu.RUnlock()

	kvNew := make(map[string]any, len(m.kv))
	fieldCacheNew := make(map[string]string, len(m.fieldCache))
	dirtyKeysNew := make(map[string]struct{}, len(m.dirtyKeys))

	maps.Copy(kvNew, m.kv)
	maps.Copy(fieldCacheNew, m.fieldCache)
	maps.Copy(dirtyKeysNew, m.dirtyKeys)

	return &CtxMap{
		kv:                   kvNew,
		prefix:               m.prefix,
		keySep:               m.keySep,
		fieldSep:             m.fieldSep,
		stringer:             m.stringer,
		prefixWithSep:        m.prefixWithSep,
		fieldCache:           fieldCacheNew,
		dirtyKeys:            dirtyKeysNew,
		currentGeneration:    m.currentGeneration,
		lastStringResult:     m.lastStringResult,
		lastStringGeneration: m.lastStringGeneration,
	}
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
	m.mu.RLock()

	if len(m.kv) == 0 {
		m.mu.RUnlock()
		return ""
	}

	// Fast path: if nothing has changed, return the cached full string.
	if m.lastStringResult != "" && m.lastStringGeneration == m.currentGeneration {
		result := m.lastStringResult
		m.mu.RUnlock()
		return result
	}

	m.mu.RUnlock()
	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check after acquiring write lock
	if m.lastStringResult != "" && m.lastStringGeneration == m.currentGeneration {
		return m.lastStringResult
	}

	// Only recalculate dirty fields and reuse cached values for others
	formattedParts := make([]string, 0, len(m.kv))
	prefix := m.prefixWithSep

	for k, v := range m.kv {
		if _, isDirty := m.dirtyKeys[k]; isDirty {
			// Recalculate and update cache for this dirty key
			formatted := m.stringer(prefix+k, v)
			m.fieldCache[k] = formatted
			formattedParts = append(formattedParts, formatted)
		} else {
			// Use existing cached value
			formattedParts = append(formattedParts, m.fieldCache[k])
		}
	}

	// All dirty keys are now clean
	m.dirtyKeys = map[string]struct{}{}

	// Build and cache the final string
	result := strings.Join(formattedParts, m.fieldSep)
	m.lastStringResult = result
	m.lastStringGeneration = m.currentGeneration

	return result
}

// markDirty is a new internal helper to handle cache invalidation.
// This method assumes the write lock is already held.
func (m *CtxMap) markDirty(key string) {
	if m.dirtyKeys == nil {
		m.dirtyKeys = map[string]struct{}{}
	}
	m.dirtyKeys[key] = struct{}{}
	m.currentGeneration++
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
