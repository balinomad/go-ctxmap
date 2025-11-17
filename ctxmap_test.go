package ctxmap

import (
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
)

// TestNewCtxMap tests the creation of a new CtxMap and the behavior of the default or custom stringer.
func TestNewCtxMap(t *testing.T) {
	// Comparing function pointers in `want` is not reliable.
	// We will test the behavior of the stringer instead of deep equality of the struct.
	t.Run("with default stringer", func(t *testing.T) {
		got := NewCtxMap(".", " ", nil)
		if got.keySep != "." || got.fieldSep != " " {
			t.Errorf("NewCtxMap() separators incorrect, got keySep=%s, fieldSep=%s", got.keySep, got.fieldSep)
		}
		if got.stringer == nil {
			t.Fatal("NewCtxMap() with nil stringer resulted in a nil stringer field")
		}
		// Test the behavior of the default stringer
		if s := got.stringer("key", "val"); s != "key=val" {
			t.Errorf("Default stringer produced wrong output: got %s, want key=val", s)
		}
	})

	t.Run("with custom stringer", func(t *testing.T) {
		customStringer := func(k string, v any) string { return fmt.Sprintf("%s->%v", k, v) }
		got := NewCtxMap(":", ";", customStringer)

		if got.stringer == nil {
			t.Fatal("NewCtxMap() with custom stringer resulted in a nil stringer field")
		}
		// Test the behavior of the custom stringer
		if s := got.stringer("key", "val"); s != "key->val" {
			t.Errorf("Custom stringer produced wrong output: got %s, want key->val", s)
		}
	})
}

// TestCtxMap_Get tests the Get() method of a CtxMap.
//
// It ensures that existing keys return the correct value and non-existent keys return nil.
// It also tests the return value of the second boolean argument, which indicates whether the key was found in the map.
// The test covers cases with existing keys, non-existent keys, and empty maps.
func TestCtxMap_Get(t *testing.T) {
	// Helper to create a map with test data
	createTestMap := func(data map[string]any) *CtxMap {
		m := NewCtxMap(".", " ", nil)
		for k, v := range data {
			m.Set(k, v)
		}
		return m
	}

	type args struct {
		key string
	}
	tests := []struct {
		name     string
		testData map[string]any
		args     args
		want     any
		want1    bool
	}{
		{
			name:     "get existing key",
			testData: map[string]any{"a": 1, "b": "hello"},
			args:     args{key: "a"},
			want:     1,
			want1:    true,
		},
		{
			name:     "get non-existent key",
			testData: map[string]any{"a": 1, "b": "hello"},
			args:     args{key: "c"},
			want:     nil,
			want1:    false,
		},
		{
			name:     "get from empty map",
			testData: nil,
			args:     args{key: "a"},
			want:     nil,
			want1:    false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := createTestMap(tt.testData)
			got, got1 := m.Get(tt.args.key)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("CtxMap.Get() got = %v, want %v", got, tt.want)
			}
			if got1 != tt.want1 {
				t.Errorf("CtxMap.Get() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}

// TestCtxMap_GetPrefixed tests the GetPrefixed() method of a CtxMap.
//
// It ensures that existing keys with the correct prefix return the correct value and non-existent keys return nil.
// It also tests the return value of the second boolean argument, which indicates whether the key was found in the map.
// The test covers cases with existing keys, non-existent keys, and empty maps.
func TestCtxMap_GetPrefixed(t *testing.T) {
	// Helper to create a map with test data and configuration
	createTestMap := func(data map[string]any, prefix, keySep string) *CtxMap {
		m := NewCtxMap(keySep, " ", nil)
		for k, v := range data {
			m.Set(k, v)
		}
		return m.WithPrefix(prefix)
	}

	type args struct {
		key string
	}
	tests := []struct {
		name     string
		testData map[string]any
		prefix   string
		keySep   string
		args     args
		want     any
		want1    bool
	}{
		{
			name:     "get with correct prefix",
			testData: map[string]any{"key1": 100},
			prefix:   "p1",
			keySep:   ".",
			args:     args{key: "p1.key1"},
			want:     100,
			want1:    true,
		},
		{
			name:     "get with incorrect prefix",
			testData: map[string]any{"key1": 100},
			prefix:   "p1",
			keySep:   ".",
			args:     args{key: "p2.key1"},
			want:     nil,
			want1:    false,
		},
		{
			name:     "get non-existent key with correct prefix",
			testData: map[string]any{"key1": 100},
			prefix:   "p1",
			keySep:   ".",
			args:     args{key: "p1.key2"},
			want:     nil,
			want1:    false,
		},
		{
			name:     "get without prefix on a prefixed map",
			testData: map[string]any{"key1": 100},
			prefix:   "p1",
			keySep:   ".",
			args:     args{key: "key1"},
			want:     nil,
			want1:    false,
		},
		{
			name:     "get on a map with no prefix",
			testData: map[string]any{"key1": 100},
			prefix:   "",
			keySep:   ".",
			args:     args{key: "key1"},
			want:     100,
			want1:    true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := createTestMap(tt.testData, tt.prefix, tt.keySep)
			got, got1 := m.GetPrefixed(tt.args.key)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("CtxMap.GetPrefixed() got = %v, want %v", got, tt.want)
			}
			if got1 != tt.want1 {
				t.Errorf("CtxMap.GetPrefixed() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}

// TestCtxMap_Set tests the Set() method of a CtxMap.
//
// It ensures that new key-value pairs are added correctly and existing keys are overwritten correctly.
// The test covers cases with setting new keys and overwriting existing keys.
func TestCtxMap_Set(t *testing.T) {
	tests := []struct {
		name string
		ops  []struct {
			key string
			val any
		}
		want map[string]any
	}{
		{
			name: "set new key",
			ops: []struct {
				key string
				val any
			}{
				{"a", 1},
			},
			want: map[string]any{"a": 1},
		},
		{
			name: "overwrite existing key",
			ops: []struct {
				key string
				val any
			}{
				{"a", 1},
				{"a", 2},
			},
			want: map[string]any{"a": 2},
		},
		{
			name: "overwrite existing key with same value",
			ops: []struct {
				key string
				val any
			}{
				{"a", 1},
				{"a", 1},
			},
			want: map[string]any{"a": 1},
		},
		{
			name: "overwrite existing key with different type",
			ops: []struct {
				key string
				val any
			}{
				{"a", 1},
				{"a", uint64(1)},
			},
			want: map[string]any{"a": uint64(1)},
		},
		{
			name: "set multiple different keys",
			ops: []struct {
				key string
				val any
			}{
				{"a", 1},
				{"b", "hello"},
				{"c", true},
			},
			want: map[string]any{"a": 1, "b": "hello", "c": true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewCtxMap(".", " ", nil)

			for _, op := range tt.ops {
				m.Set(op.key, op.val)
			}

			for k, wantVal := range tt.want {
				gotVal, ok := m.Get(k)
				if !ok || gotVal != wantVal {
					t.Errorf("expected key=%q to have val=%v, got val=%v, ok=%t",
						k, wantVal, gotVal, ok)
				}
			}
		})
	}
}

func TestCtxMap_SetMultiple(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(m *CtxMap)
		input     map[string]any
		want      map[string]any
		wantDirty []string
	}{
		{
			name:  "empty input does nothing",
			input: map[string]any{},
			setup: nil,
			want:  map[string]any{},
			// no dirty keys expected
			wantDirty: []string{},
		},
		{
			name: "set new keys",
			input: map[string]any{
				"a": 1,
				"b": "hello",
			},
			want: map[string]any{
				"a": 1,
				"b": "hello",
			},
			wantDirty: []string{"a", "b"},
		},
		{
			name: "overwrite existing key with new value",
			setup: func(m *CtxMap) {
				m.Set("a", 1)
			},
			input: map[string]any{
				"a": 2,
			},
			want: map[string]any{
				"a": 2,
			},
			wantDirty: []string{"a"},
		},
		{
			name: "overwrite existing key with same value (always marks dirty for safety)",
			setup: func(m *CtxMap) {
				m.Set("a", 1)
			},
			input: map[string]any{
				"a": 1,
			},
			want: map[string]any{
				"a": 1,
			},
			// The safe implementation *must* mark it as dirty.
			wantDirty: []string{"a"},
		},
		{
			name: "overwrite with different type",
			setup: func(m *CtxMap) {
				m.Set("a", 1)
			},
			input: map[string]any{
				"a": uint64(1),
			},
			want: map[string]any{
				"a": uint64(1),
			},
			wantDirty: []string{"a"},
		},
		{
			name: "set multiple keys at once",
			setup: func(m *CtxMap) {
				m.Set("x", true)
			},
			input: map[string]any{
				"a": 1,
				"b": "world",
				"x": false, // overwrite existing
			},
			want: map[string]any{
				"a": 1,
				"b": "world",
				"x": false,
			},
			wantDirty: []string{"a", "b", "x"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewCtxMap(".", " ", nil)

			if tt.setup != nil {
				tt.setup(m)
				// reset dirtyKeys from setup so only SetMultiple effects are tracked
				m.dirtyKeys = make(map[string]struct{})
			}

			m.SetMultiple(tt.input)

			// Verify final state
			for k, wantVal := range tt.want {
				gotVal, ok := m.Get(k)
				if !ok || gotVal != wantVal {
					t.Errorf("expected key=%q to have val=%v, got val=%v (ok=%t)", k, wantVal, gotVal, ok)
				}
			}

			// Verify dirty keys
			gotDirty := make([]string, 0, len(m.dirtyKeys))
			for k := range m.dirtyKeys {
				gotDirty = append(gotDirty, k)
			}
			sort.Strings(gotDirty)
			sort.Strings(tt.wantDirty)

			if !reflect.DeepEqual(gotDirty, tt.wantDirty) {
				t.Errorf("dirty keys mismatch: want=%v, got=%v", tt.wantDirty, gotDirty)
			}
		})
	}
}

// TestCtxMap_Delete tests the Delete() method of a CtxMap.
//
// It ensures that existing keys are removed correctly and non-existent keys do not change the map.
// The test covers cases with deleting existing keys and non-existent keys.
func TestCtxMap_Delete(t *testing.T) {
	// This test needs to verify the state of the map after the operation.
	t.Run("delete existing key", func(t *testing.T) {
		m := NewCtxMap(".", " ", nil)
		m.Set("a", 1)
		m.Set("b", 2)
		m.Delete("a")
		if _, ok := m.Get("a"); ok {
			t.Error("Delete failed, key 'a' should not exist")
		}
		if m.Len() != 1 {
			t.Errorf("Delete failed, expected map length 1, got %d", m.Len())
		}
	})

	t.Run("delete non-existent key", func(t *testing.T) {
		m := NewCtxMap(".", " ", nil)
		m.Set("a", 1)
		m.Delete("b")
		if m.Len() != 1 {
			t.Errorf("Delete non-existent key should not change map length, got %d", m.Len())
		}
	})
}

// TestCtxMap_DeletePrefixed tests the DeletePrefixed() method of a CtxMap.
//
// It ensures that existing keys are removed correctly and non-existent keys do not change the map.
// The test covers cases with deleting existing keys and non-existent keys.
func TestCtxMap_DeletePrefixed(t *testing.T) {
	// This test needs to verify the state of the map after the operation.
	t.Run("delete with correct prefix", func(t *testing.T) {
		m := NewCtxMap(".", " ", nil).WithPrefix("p1")
		m.Set("key1", 1)
		m.DeletePrefixed("p1.key1")
		if m.Len() != 0 {
			t.Errorf("DeletePrefixed failed, expected map length 0, got %d", m.Len())
		}
	})

	t.Run("delete with incorrect prefix", func(t *testing.T) {
		m := NewCtxMap(".", " ", nil).WithPrefix("p1")
		m.Set("key1", 1)
		m.DeletePrefixed("p2.key1")
		if m.Len() != 1 {
			t.Errorf("DeletePrefixed with wrong prefix should not change map length, got %d", m.Len())
		}
	})

	t.Run("delete on map with no prefix", func(t *testing.T) {
		m := NewCtxMap(".", " ", nil)
		m.Set("key1", 1)
		m.DeletePrefixed("key1")
		if m.Len() != 0 {
			t.Errorf("DeletePrefixed on non-prefixed map failed, expected length 0, got %d", m.Len())
		}
	})
}

// TestCtxMap_WithPairs tests the WithPairs() method of a CtxMap.
//
// It ensures that new key-value pairs are added correctly and incomplete pairs are skipped.
// The test covers cases with adding new pairs, incomplete pairs, and no pairs.
func TestCtxMap_WithPairs(t *testing.T) {
	baseMap := NewCtxMap(".", " ", nil)
	baseMap.Set("a", 1)

	type args struct {
		keyValues []any
	}
	tests := []struct {
		name string
		args args
		want map[string]any
	}{
		{
			name: "add new pairs",
			args: args{keyValues: []any{"b", 2, "c", 3}},
			want: map[string]any{"a": 1, "b": 2, "c": 3},
		},
		{
			name: "incomplete pair",
			args: args{keyValues: []any{"b", 2, "c"}},
			want: map[string]any{"a": 1, "b": 2},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a fresh map for each test run to avoid mutation issues
			m := NewCtxMap(".", " ", nil)
			m.Set("a", 1)

			got := m.WithPairs(tt.args.keyValues...)

			// Check the resulting map data
			gotData := got.ToMapCopy()
			if !reflect.DeepEqual(gotData, tt.want) {
				t.Errorf("CtxMap.WithPairs() = %v, want %v", gotData, tt.want)
			}
			// Ensure original map is not modified
			if m.Len() != 1 {
				t.Error("Original map was modified by WithPairs()")
			}
		})
	}

	t.Run("no pairs", func(t *testing.T) {
		m := NewCtxMap(".", " ", nil)
		m.Set("a", 1)

		got := m.WithPairs()

		// Should return the same instance for empty keyValues
		if got != m {
			t.Errorf("WithPairs() with no pairs should return the same instance")
		}
	})
}

// TestCtxMap_ReplaceAll tests the ReplaceAll() method of a CtxMap.
//
// It ensures that ReplaceAll() correctly replaces all key-value pairs in the map.
// The test covers cases with replacing an existing map and replacing with an empty slice.
func TestCtxMap_ReplaceAll(t *testing.T) {
	t.Run("replace existing map", func(t *testing.T) {
		m := NewCtxMap(".", " ", nil)
		m.Set("a", 1)
		m.ReplaceAll("b", 2, "c", 3)
		expected := map[string]any{"b": 2, "c": 3}
		actual := m.ToMapCopy()
		if !reflect.DeepEqual(actual, expected) {
			t.Errorf("ReplaceAll() got %v, want %v", actual, expected)
		}
	})

	t.Run("replace with empty slice", func(t *testing.T) {
		m := NewCtxMap(".", " ", nil)
		m.Set("a", 1)
		m.ReplaceAll()
		if m.Len() != 0 {
			t.Errorf("ReplaceAll() with empty slice should result in an empty map, got len %d", m.Len())
		}
	})
}

// TestCtxMap_Clear tests the Clear() method of a CtxMap.
//
// It ensures that Clear() correctly removes all key-value pairs from the map.
// The test covers cases with clearing a non-empty map.
func TestCtxMap_Clear(t *testing.T) {
	t.Run("clear a non-empty map", func(t *testing.T) {
		m := NewCtxMap(".", " ", nil)
		m.Set("a", 1)
		m.Clear()
		if m.Len() != 0 {
			t.Errorf("Clear() should result in an empty map, got len %d", m.Len())
		}
	})
}

// TestCtxMap_WithPrefix tests the WithPrefix() method of a CtxMap.
//
// It ensures that WithPrefix() correctly returns a new CtxMap with an added prefix.
// The test covers cases with adding a prefix, adding a nested prefix, and returning the same instance with an empty prefix.
func TestCtxMap_WithPrefix(t *testing.T) {
	baseMap := NewCtxMap(".", " ", nil)
	baseMap.Set("key", "val")

	t.Run("add a prefix", func(t *testing.T) {
		got := baseMap.WithPrefix("p1")
		if got.prefix != "p1" {
			t.Errorf("WithPrefix() prefix = %v, want %v", got.prefix, "p1")
		}
		// With RWMutex implementation, we cannot share the underlying map
		// because we cannot share mutexes safely. This behavior change is expected.
		// Verify the data is correctly copied instead
		if val, ok := got.Get("key"); !ok || val != "val" {
			t.Errorf("WithPrefix() should preserve data, got val=%v, ok=%t", val, ok)
		}
	})

	t.Run("add a nested prefix", func(t *testing.T) {
		got := baseMap.WithPrefix("p1").WithPrefix("p2")
		if got.prefix != "p1.p2" {
			t.Errorf("WithPrefix() nested prefix = %v, want %v", got.prefix, "p1.p2")
		}
	})

	t.Run("empty prefix returns same instance", func(t *testing.T) {
		got := baseMap.WithPrefix("")
		if got != baseMap {
			t.Error("WithPrefix(\"\") should return the same instance")
		}
	})
}

// TestCtxMap_Merge tests the Merge() method of a CtxMap.
//
// It ensures that Merge() correctly returns a new CtxMap containing pairs from both maps.
// The test covers cases with merging two maps, preserving the original maps, and handling duplicate keys.
func TestCtxMap_Merge(t *testing.T) {
	mapA := NewCtxMap(".", " ", nil)
	mapA.Set("a", 1)
	mapA.Set("b", 2)

	mapB := NewCtxMap(".", " ", nil)
	mapB.Set("b", 99)
	mapB.Set("c", 3)

	t.Run("merge two maps", func(t *testing.T) {
		got := mapA.Merge(mapB)
		expected := map[string]any{"a": 1, "b": 99, "c": 3}
		actual := got.ToMapCopy()
		if !reflect.DeepEqual(actual, expected) {
			t.Errorf("Merge() = %v, want %v", actual, expected)
		}
		// Ensure original maps are not modified
		if mapA.Len() != 2 || mapB.Len() != 2 {
			t.Error("Original maps were modified by Merge()")
		}
	})
}

// TestCtxMap_Clone tests the Clone() method of a CtxMap.
//
// It ensures that Clone() correctly returns a new CtxMap with the same data as the original map.
func TestCtxMap_Clone(t *testing.T) {
	t.Run("clone a map", func(t *testing.T) {
		original := NewCtxMap(".", " ", nil).WithPrefix("p1")
		original.Set("a", 1)

		clone := original.Clone()

		originalData := original.ToMapCopy()
		cloneData := clone.ToMapCopy()
		if !reflect.DeepEqual(originalData, cloneData) {
			t.Error("Clone() data should be equal")
		}

		if original.prefix != clone.prefix {
			t.Errorf("Clone() prefix mismatch, got %s, want %s", clone.prefix, original.prefix)
		}

		// Modify clone and check original
		clone.Set("b", 2)
		if _, ok := original.Get("b"); ok {
			t.Error("Modifying clone affected the original map")
		}
	})
}

// TestCtxMap_AsMap tests the AsMap() method of a CtxMap.
//
// It ensures that AsMap() correctly returns a snapshot of the map with prefixes applied.
// The test covers cases with and without prefixes.
func TestCtxMap_AsMap(t *testing.T) {
	tests := []struct {
		name     string
		testData map[string]any
		prefix   string
		keySep   string
		want     map[string]any
	}{
		{
			name:     "map without prefix",
			testData: map[string]any{"a": 1},
			prefix:   "",
			keySep:   ".",
			want:     map[string]any{"a": 1},
		},
		{
			name:     "map with prefix",
			testData: map[string]any{"a": 1},
			prefix:   "p1",
			keySep:   ".",
			want:     map[string]any{"p1.a": 1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewCtxMap(tt.keySep, " ", nil)
			for k, v := range tt.testData {
				m.Set(k, v)
			}
			if tt.prefix != "" {
				m = m.WithPrefix(tt.prefix)
			}

			if got := m.AsMap(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("CtxMap.AsMap() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestCtxMap_ToMapCopy tests the ToMapCopy() method of a CtxMap.
//
// It tests that ToMapCopy() returns a new map containing a snapshot of the data with prefixes applied,
// and that modifying the copy does not affect the original map.
// It also tests that ToMapCopy() works correctly with prefixes.
func TestCtxMap_ToMapCopy(t *testing.T) {
	originalMap := NewCtxMap(".", " ", nil)
	originalMap.Set("a", 1)

	t.Run("copy and modify", func(t *testing.T) {
		copiedMap := originalMap.ToMapCopy()
		copiedMap["b"] = 2 // Modify the copy

		if _, ok := originalMap.Get("b"); ok {
			t.Error("Modifying the copy from ToMapCopy() affected the original map")
		}
	})

	t.Run("copy with prefix", func(t *testing.T) {
		prefixedMap := originalMap.WithPrefix("p1")
		copiedMap := prefixedMap.ToMapCopy()

		expected := map[string]any{"p1.a": 1}
		if !reflect.DeepEqual(copiedMap, expected) {
			t.Errorf("ToMapCopy() with prefix = %v, want %v", copiedMap, expected)
		}
	})
}

// sortablePairs helps sort a flat slice of key-value pairs.
type sortablePairs struct {
	slice []any
}

func (p sortablePairs) Len() int { return len(p.slice) / 2 }
func (p sortablePairs) Swap(i, j int) {
	// Calculate the actual slice indices for the pairs
	i_key, j_key := i*2, j*2
	i_val, j_val := i*2+1, j*2+1

	// Swap keys
	p.slice[i_key], p.slice[j_key] = p.slice[j_key], p.slice[i_key]
	// Swap values
	p.slice[i_val], p.slice[j_val] = p.slice[j_val], p.slice[i_val]
}
func (p sortablePairs) Less(i, j int) bool {
	// Compare keys at the calculated indices
	return fmt.Sprint(p.slice[i*2]) < fmt.Sprint(p.slice[j*2])
}

// TestCtxMap_ToSliceCopy tests the ToSliceCopy() method of a CtxMap.
//
// It ensures that the returned slice contains all the key-value pairs from the map,
// with the prefix applied as needed. The test covers cases with and without prefixes.
// The test also ensures that the returned slice is independent of the original map,
// and that modifying the copy does not affect the original map.
func TestCtxMap_ToSliceCopy(t *testing.T) {
	// Since map iteration order is not guaranteed, we sort the slices for stable comparison.
	sorter := func(s []any) {
		if len(s) < 2 {
			return
		}
		sort.Sort(sortablePairs{slice: s})
	}

	tests := []struct {
		name     string
		testData map[string]any
		prefix   string
		keySep   string
		want     []any
	}{
		{
			name:     "slice without prefix",
			testData: map[string]any{"b": 2, "a": 1},
			prefix:   "",
			keySep:   ".",
			want:     []any{"a", 1, "b", 2},
		},
		{
			name:     "slice with prefix",
			testData: map[string]any{"b": 2, "a": 1},
			prefix:   "p",
			keySep:   ".",
			want:     []any{"p.a", 1, "p.b", 2},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewCtxMap(tt.keySep, " ", nil)
			for k, v := range tt.testData {
				m.Set(k, v)
			}
			if tt.prefix != "" {
				m = m.WithPrefix(tt.prefix)
			}

			got := m.ToSliceCopy()

			// Sort both slices to ensure order doesn't affect the test
			sorter(got)
			sorter(tt.want)

			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("CtxMap.ToSliceCopy() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestCtxMap_Len tests the Len() method of a CtxMap.
//
// It ensures that Len() correctly returns the number of key-value pairs in the map.
// The test covers cases with empty maps and maps with items.
func TestCtxMap_Len(t *testing.T) {
	tests := []struct {
		name     string
		testData map[string]any
		want     int
	}{
		{
			name:     "empty map",
			testData: nil,
			want:     0,
		},
		{
			name:     "map with items",
			testData: map[string]any{"a": 1, "b": 2},
			want:     2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := NewCtxMap(".", " ", nil)
			for k, v := range tt.testData {
				m.Set(k, v)
			}
			if got := m.Len(); got != tt.want {
				t.Errorf("CtxMap.Len() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestCtxMap_Range tests the Range() method of a CtxMap.
//
// It ensures that Range() correctly iterates over the map, applying the prefix to keys.
// The test covers cases with and without prefixes.
//
// The test also verifies that the passed callback function is called for each key-value pair in the map.
func TestCtxMap_Range(t *testing.T) {
	t.Run("range over map with prefix", func(t *testing.T) {
		m := NewCtxMap(".", " ", nil).WithPrefix("p")
		m.Set("a", 1)
		m.Set("b", 2)

		results := make(map[string]any)
		m.Range(func(k string, v any) {
			results[k] = v
		})

		expected := map[string]any{"p.a": 1, "p.b": 2}
		if !reflect.DeepEqual(results, expected) {
			t.Errorf("Range() results = %v, want %v", results, expected)
		}
	})

	t.Run("range over map without prefix", func(t *testing.T) {
		m := NewCtxMap(".", " ", nil)
		m.SetMultiple(map[string]any{"a": 1, "b": uint(2)})

		results := make(map[string]any)
		m.Range(func(k string, v any) {
			results[k] = v
		})

		expected := map[string]any{"a": 1, "b": uint(2)}
		if !reflect.DeepEqual(results, expected) {
			t.Errorf("Range() results = %v, want %v", results, expected)
		}
	})
}

// TestCtxMap_String tests the String() method of a CtxMap.
//
// It ensures that the correct output is generated, in the correct format, and that the
// order of key-value pairs does not affect the result.
func TestCtxMap_String(t *testing.T) {
	m := NewCtxMap(".", " ", nil)
	m.Set("a", 1)
	m.Set("b", 2)

	got := m.String()

	// To validate without depending on order, we split the output
	// and check that all expected parts are present.
	parts := strings.Split(got, " ")
	if len(parts) != 2 {
		t.Fatalf("String() expected 2 parts, got %d from output %q", len(parts), got)
	}

	// Use a map to easily check for the presence of each part.
	gotParts := make(map[string]bool)
	for _, p := range parts {
		gotParts[p] = true
	}

	expectedParts := []string{"a=1", "b=2"}
	for _, p := range expectedParts {
		if !gotParts[p] {
			t.Errorf("String() output %q is missing expected part %q", got, p)
		}
	}
}

// TestCtxMap_String_WithPrefix tests the String() method of a CtxMap with a prefix.
//
// It ensures that the correct output is generated, in the correct format, and that the
// order of key-value pairs does not affect the result.
// The test covers cases with and without a prefix.
func TestCtxMap_String_WithPrefix(t *testing.T) {
	m := NewCtxMap(".", " ", nil).WithPrefix("pre")
	m.Set("a", 1)
	m.Set("b", 2)

	got := m.String()
	parts := strings.Split(got, " ")
	if len(parts) != 2 {
		t.Fatalf("String() with prefix expected 2 parts, got %d from output %q", len(parts), got)
	}

	gotParts := make(map[string]bool)
	for _, p := range parts {
		gotParts[p] = true
	}

	expectedParts := []string{"pre.a=1", "pre.b=2"}
	for _, p := range expectedParts {
		if !gotParts[p] {
			t.Errorf("String() with prefix output %q is missing expected part %q", got, p)
		}
	}
}

// TestCtxMap_String_Empty tests that the String() method returns an empty string for an empty CtxMap.
func TestCtxMap_String_Empty(t *testing.T) {
	m := NewCtxMap(".", " ", nil)
	got := m.String()
	if got != "" {
		t.Errorf("String() on empty map should return empty string, got %q", got)
	}
}

// TestCtxMap_String_Caching verifies the caching mechanism of the String() method.
// It ensures that the "fast path" is hit when the map is unchanged, and that the
// cache is correctly invalidated and updated after a modification.
func TestCtxMap_String_Caching(t *testing.T) {
	m := NewCtxMap(".", " ", nil)
	m.Set("b", "2")
	m.Set("a", "1")

	// Helper to check the content of the string output regardless of field order.
	checkStringContent := func(t *testing.T, got string, wantParts ...string) {
		t.Helper()
		gotParts := strings.Split(got, " ")
		sort.Strings(gotParts)
		sort.Strings(wantParts)
		if !reflect.DeepEqual(gotParts, wantParts) {
			t.Errorf("String() content mismatch:\n got: %v\nwant: %v", gotParts, wantParts)
		}
	}

	// First call to String(). This populates the cache (slow path)
	firstResult := m.String()
	checkStringContent(t, firstResult, "a=1", "b=2")

	// Second call to String(). This MUST hit the fast path and return the cached value
	secondResult := m.String()
	if secondResult != firstResult {
		t.Errorf("Second call should return cached string, but it was different.\nGot: %q\nWant: %q", secondResult, firstResult)
	}

	// Modifying the map must invalidate the cache
	m.Set("c", "3")

	// The tird call to String() must use the slow path again to generate a new string
	thirdResult := m.String()
	checkStringContent(t, thirdResult, "a=1", "b=2", "c=3")

	// And finally, this is the fourth call to String() that must hit the fast path again
	// with the new cached value
	fourthResult := m.String()
	if fourthResult != thirdResult {
		t.Errorf("Fourth call should return new cached string, but it was different.\nGot: %q\nWant: %q", fourthResult, thirdResult)
	}
}

// TestCtxMap_markDirty tests the markDirty() method of a CtxMap.
// It ensures that markDirty() correctly marks keys as dirty and
// increments the current generation.
func TestCtxMap_markDirty(t *testing.T) {
	tests := []struct {
		name              string
		setup             func(m *CtxMap)
		key               string
		wantDirtyKeys     []string
		wantGenerationInc int // how many times generation should increase
	}{
		{
			name: "dirtyKeys is nil, first key added",
			setup: func(m *CtxMap) {
				m.dirtyKeys = nil
				m.currentGeneration.Store(0)
			},
			key:               "a",
			wantDirtyKeys:     []string{"a"},
			wantGenerationInc: 1,
		},
		{
			name: "add new key to existing dirtyKeys",
			setup: func(m *CtxMap) {
				m.dirtyKeys = map[string]struct{}{"a": {}}
				m.currentGeneration.Store(5)
			},
			key:               "b",
			wantDirtyKeys:     []string{"a", "b"},
			wantGenerationInc: 1,
		},
		{
			name: "add duplicate key (already dirty)",
			setup: func(m *CtxMap) {
				m.dirtyKeys = map[string]struct{}{"a": {}}
				m.currentGeneration.Store(10)
			},
			key:               "a",
			wantDirtyKeys:     []string{"a"}, // no duplicate
			wantGenerationInc: 1,             // still increments
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &CtxMap{}

			// Run setup
			tt.setup(m)
			initialGen := m.currentGeneration.Load()

			// Call markDirty
			m.markDirty(tt.key)

			// Collect actual dirty keys
			gotDirty := make([]string, 0, len(m.dirtyKeys))
			for k := range m.dirtyKeys {
				gotDirty = append(gotDirty, k)
			}
			sort.Strings(gotDirty)
			sort.Strings(tt.wantDirtyKeys)

			if !reflect.DeepEqual(gotDirty, tt.wantDirtyKeys) {
				t.Errorf("dirtyKeys mismatch: want=%v, got=%v", tt.wantDirtyKeys, gotDirty)
			}

			// Check generation increment
			gotInc := int(m.currentGeneration.Load() - initialGen)
			if gotInc != tt.wantGenerationInc {
				t.Errorf("currentGeneration increment mismatch: want=%d, got=%d", tt.wantGenerationInc, gotInc)
			}
		})
	}
}

// Test_toString tests the toString() function to ensure it correctly converts
// a given value into a string. The test covers cases with strings, integers,
// booleans, and nil values.
func Test_toString(t *testing.T) {
	type args struct {
		v any
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{name: "string", args: args{v: "hello"}, want: "hello"},
		{name: "int", args: args{v: 123}, want: "123"},
		{name: "int64", args: args{v: int64(-456)}, want: "-456"},
		{name: "uint", args: args{v: uint(123)}, want: "123"},
		{name: "uint64", args: args{v: uint64(789)}, want: "789"},
		{name: "bool true", args: args{v: true}, want: "true"},
		{name: "bool false", args: args{v: false}, want: "false"},
		{name: "nil", args: args{v: nil}, want: "<nil>"},
		{name: "default case float", args: args{v: 3.14}, want: "3.14"},
		{name: "default case struct", args: args{v: struct{ X int }{X: 1}}, want: "{1}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := toString(tt.args.v); got != tt.want {
				t.Errorf("toString(%v) = %v, want %v", tt.args.v, got, tt.want)
			}
		})
	}
}

func Test_itoa64(t *testing.T) {
	tests := []struct {
		name string
		val  int64
		want string
	}{
		{name: "zero", val: 0, want: "0"},
		{name: "positive single digit", val: 7, want: "7"},
		{name: "positive multi digit", val: 12345, want: "12345"},
		{name: "negative single digit", val: -9, want: "-9"},
		{name: "negative multi digit", val: -98765, want: "-98765"},
		{name: "max int64", val: math.MaxInt64, want: "9223372036854775807"},
		{name: "min int64 (overflow case)", val: math.MinInt64, want: "-9223372036854775808"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := itoa64(tt.val)
			if got != tt.want {
				t.Errorf("itoa64(%d) = %q, want %q", tt.val, got, tt.want)
			}
		})
	}
}

func Test_utoa64(t *testing.T) {
	tests := []struct {
		name string
		val  uint64
		want string
	}{
		{name: "zero", val: 0, want: "0"},
		{name: "single digit", val: 7, want: "7"},
		{name: "multi digit", val: 1234567890, want: "1234567890"},
		{name: "max uint64", val: math.MaxUint64, want: "18446744073709551615"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := utoa64(tt.val)
			if got != tt.want {
				t.Errorf("utoa64(%d) = %q, want %q", tt.val, got, tt.want)
			}
		})
	}
}

// Test_joinPrefix tests the joinPrefix() function to ensure it correctly combines
// two prefixes with a separator, and handles edge cases where one or both prefixes
// are empty strings.
func Test_joinPrefix(t *testing.T) {
	type args struct {
		existing  string
		new       string
		separator string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{name: "both present", args: args{"a", "b", "."}, want: "a.b"},
		{name: "existing empty", args: args{"", "b", "."}, want: "b"},
		{name: "new empty", args: args{"a", "", "."}, want: "a"},
		{name: "both empty", args: args{"", "", "."}, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := joinPrefix(tt.args.existing, tt.args.new, tt.args.separator); got != tt.want {
				t.Errorf("joinPrefix() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Test for concurrency safety.
func TestCtxMap_Concurrency(t *testing.T) {
	m := NewCtxMap(".", " ", nil)
	var wg sync.WaitGroup
	numRoutines := 50
	numWrites := 50

	// Concurrent writes
	for i := 0; i < numRoutines; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < numWrites; j++ {
				key := fmt.Sprintf("key-%d-%d", i, j)
				m.Set(key, i*j)
			}
		}(i)
	}

	// Concurrent reads
	for i := 0; i < numRoutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			m.Range(func(k string, v any) {
				// Read operation
			})
			_ = m.String()
			_ = m.AsMap()
		}()
	}

	wg.Wait()

	expectedLen := numRoutines * numWrites
	if m.Len() != expectedLen {
		t.Errorf("expected final length %d, got %d", expectedLen, m.Len())
	}
}
