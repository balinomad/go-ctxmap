package ctxmap

import (
	"fmt"
	"runtime"
	"strconv"
	"sync"
	"testing"
)

// Number of key-value pairs to generate (none, average, high)
var keySizes = []int{2, 10, 100, 1000}

// Benchmark data generation helpers
func generateKeys(n int, prefix string) []string {
	keys := make([]string, n)
	for i := 0; i < n; i++ {
		keys[i] = fmt.Sprintf("%s%d", prefix, i)
	}
	return keys
}

func generateKeyValuePairs(n int, prefix string) []any {
	pairs := make([]any, 0, n*2)
	for i := 0; i < n; i++ {
		pairs = append(pairs, fmt.Sprintf("%s%d", prefix, i), i)
	}
	return pairs
}

func createPopulatedMap(n int) *CtxMap {
	m := NewCtxMap(".", " ", nil)
	for i := 0; i < n; i++ {
		m.Set(fmt.Sprintf("key%d", i), i)
	}
	return m
}

// Basic Operations Benchmarks

func BenchmarkSet(b *testing.B) {
	for _, size := range keySizes {
		b.Run(fmt.Sprintf("size_%d", size), func(b *testing.B) {
			m := NewCtxMap(".", " ", nil)
			keys := generateKeys(size, "key")

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				idx := i % size
				m.Set(keys[idx], i)
			}
		})
	}
}

func BenchmarkGet(b *testing.B) {
	for _, size := range keySizes {
		b.Run(fmt.Sprintf("size_%d", size), func(b *testing.B) {
			m := createPopulatedMap(size)
			keys := generateKeys(size, "key")

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				idx := i % size
				_, _ = m.Get(keys[idx])
			}
		})
	}
}

func BenchmarkGetPrefixed(b *testing.B) {
	for _, size := range keySizes {
		b.Run(fmt.Sprintf("size_%d", size), func(b *testing.B) {
			m := createPopulatedMap(size).WithPrefix("prefix")
			prefixedKeys := generateKeys(size, "prefix.key")

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				idx := i % size
				_, _ = m.GetPrefixed(prefixedKeys[idx])
			}
		})
	}
}

func BenchmarkDelete(b *testing.B) {
	b.Run("delete_existing", func(b *testing.B) {
		const poolSize = 1024 // small, fixed pool instead of b.N

		// Build a pool of maps where "key50" exists
		pool := make([]*CtxMap, poolSize)
		for i := 0; i < poolSize; i++ {
			pool[i] = createPopulatedMap(100)
		}

		b.ResetTimer()
		b.ReportAllocs()

		idx := 0

		for i := 0; i < b.N; i++ {
			pool[idx].Delete("key50")
			idx++
			if idx == poolSize {
				// Restore the pool outside of timing so Set() isn’t counted
				b.StopTimer()
				for j := 0; j < poolSize; j++ {
					pool[j].Set("key50", 50)
				}
				b.StartTimer()
				idx = 0
			}
		}
	})

	b.Run("delete_non_existing", func(b *testing.B) {
		// Only need a single map; deleting a missing key is idempotent
		m := createPopulatedMap(100)

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			m.Delete("non_existing_key")
		}
	})
}

// Immutable Operations Benchmarks

func BenchmarkWithPairs(b *testing.B) {
	for _, size := range keySizes {
		b.Run(fmt.Sprintf("pairs_%d", size), func(b *testing.B) {
			baseMap := createPopulatedMap(10)
			pairs := generateKeyValuePairs(size, "new_key")

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				_ = baseMap.WithPairs(pairs...)
			}
		})
	}

}

func BenchmarkWithPrefix(b *testing.B) {
	for _, size := range keySizes {
		b.Run(fmt.Sprintf("size_%d", size), func(b *testing.B) {
			m := createPopulatedMap(size)

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				_ = m.WithPrefix("benchmark_prefix")
			}
		})
	}
}

func BenchmarkMerge(b *testing.B) {
	for _, size := range keySizes {
		b.Run(fmt.Sprintf("size_%d", size), func(b *testing.B) {
			m1 := createPopulatedMap(size)
			m2 := createPopulatedMap(size / 2)

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				_ = m1.Merge(m2)
			}
		})
	}
}

func BenchmarkClone(b *testing.B) {
	for _, size := range keySizes {
		b.Run(fmt.Sprintf("size_%d", size), func(b *testing.B) {
			m := createPopulatedMap(size)

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				_ = m.Clone()
			}
		})
	}
}

// Output Operations Benchmarks

func BenchmarkAsMap(b *testing.B) {
	b.Run("without_prefix", func(b *testing.B) {
		for _, size := range keySizes {
			b.Run(fmt.Sprintf("size_%d", size), func(b *testing.B) {
				m := createPopulatedMap(size)

				b.ResetTimer()
				b.ReportAllocs()

				for i := 0; i < b.N; i++ {
					_ = m.AsMap()
				}
			})
		}
	})

	b.Run("with_prefix", func(b *testing.B) {
		for _, size := range keySizes {
			b.Run(fmt.Sprintf("size_%d", size), func(b *testing.B) {
				m := createPopulatedMap(size).WithPrefix("bench")

				b.ResetTimer()
				b.ReportAllocs()

				for i := 0; i < b.N; i++ {
					_ = m.AsMap()
				}
			})
		}
	})
}

func BenchmarkToMapCopy(b *testing.B) {
	for _, size := range keySizes {
		b.Run(fmt.Sprintf("size_%d", size), func(b *testing.B) {
			m := createPopulatedMap(size)

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				_ = m.ToMapCopy()
			}
		})
	}
}

func BenchmarkToSliceCopy(b *testing.B) {
	for _, size := range keySizes {
		b.Run(fmt.Sprintf("size_%d", size), func(b *testing.B) {
			m := createPopulatedMap(size)

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				_ = m.ToSliceCopy()
			}
		})
	}
}

func BenchmarkString(b *testing.B) {
	b.Run("without_prefix", func(b *testing.B) {
		for _, size := range keySizes {
			b.Run(fmt.Sprintf("size_%d", size), func(b *testing.B) {
				m := createPopulatedMap(size)

				b.ResetTimer()
				b.ReportAllocs()

				for i := 0; i < b.N; i++ {
					_ = m.String()
				}
			})
		}
	})

	b.Run("with_prefix", func(b *testing.B) {
		for _, size := range keySizes {
			b.Run(fmt.Sprintf("size_%d", size), func(b *testing.B) {
				m := createPopulatedMap(size).WithPrefix("prefix")

				b.ResetTimer()
				b.ReportAllocs()

				for i := 0; i < b.N; i++ {
					_ = m.String()
				}
			})
		}
	})
}

func BenchmarkRange(b *testing.B) {
	for _, size := range keySizes {
		b.Run(fmt.Sprintf("size_%d", size), func(b *testing.B) {
			m := createPopulatedMap(size)

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				m.Range(func(k string, v any) {
					// Minimal work to avoid optimizing away the call
					_ = k
					_ = v
				})
			}
		})
	}
}

// Custom Stringer Benchmarks

func BenchmarkStringer(b *testing.B) {
	customStringer := func(k string, v any) string {
		return fmt.Sprintf(`"%s":"%v"`, k, v) // JSON-like format
	}
	for _, size := range keySizes {
		b.Run(fmt.Sprintf("size_%d", size), func(b *testing.B) {
			m := NewCtxMap(".", ",", customStringer)
			for i := 0; i < size; i++ {
				m.Set(fmt.Sprintf("key%d", i), fmt.Sprintf("value%d", i))
			}

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				_ = m.String()
			}
		})
	}
}

// Memory Usage Benchmarks

func BenchmarkMemoryUsage(b *testing.B) {
	b.Run("growth", func(b *testing.B) {
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			m := NewCtxMap(".", " ", nil)
			for j := 0; j < 1000; j++ {
				m.Set(strconv.Itoa(j), j)
			}
		}
	})

	b.Run("prefix_memory_overhead", func(b *testing.B) {
		baseMap := createPopulatedMap(100)
		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			_ = baseMap.WithPrefix("prefix").WithPrefix("nested")
		}
	})
}

// Concurrency Benchmarks

func BenchmarkConcurrentReads(b *testing.B) {
	concurrencyLevels := []int{2, 4, 8, 16}

	for _, size := range keySizes {
		for _, concurrency := range concurrencyLevels {
			b.Run(fmt.Sprintf("size_%d_goroutines_%d", size, concurrency), func(b *testing.B) {
				m := createPopulatedMap(size)
				keys := generateKeys(size, "key")

				b.ResetTimer()
				b.SetParallelism(concurrency)

				b.RunParallel(func(pb *testing.PB) {
					for pb.Next() {
						idx := runtime.NumGoroutine() % size
						if idx < 0 {
							idx = -idx
						}
						_, _ = m.Get(keys[idx])
					}
				})
			})
		}
	}
}

func BenchmarkConcurrentWrites(b *testing.B) {
	concurrencyLevels := []int{2, 4, 8}

	for _, concurrency := range concurrencyLevels {
		b.Run(fmt.Sprintf("goroutines_%d", concurrency), func(b *testing.B) {
			m := NewCtxMap(".", " ", nil)

			b.ResetTimer()
			b.SetParallelism(concurrency)

			b.RunParallel(func(pb *testing.PB) {
				i := 0
				for pb.Next() {
					key := fmt.Sprintf("key_%d_%d", runtime.NumGoroutine(), i)
					m.Set(key, i)
					i++
				}
			})
		})
	}
}

func BenchmarkConcurrentReadWrite(b *testing.B) {
	b.Run("mixed_operations", func(b *testing.B) {
		m := createPopulatedMap(1000)
		keys := generateKeys(1000, "key")

		b.ResetTimer()
		b.SetParallelism(8)

		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				if i%10 == 0 {
					// 10% writes
					key := fmt.Sprintf("new_key_%d_%d", runtime.NumGoroutine(), i)
					m.Set(key, i)
				} else {
					// 90% reads
					idx := i % len(keys)
					_, _ = m.Get(keys[idx])
				}
				i++
			}
		})
	})
}

// Specialized Logging Scenario Benchmarks

func BenchmarkLoggingScenarios(b *testing.B) {
	b.Run("typical_log_entry", func(b *testing.B) {
		// Simulates typical logging: create base context, add request-specific fields, serialize
		baseContext := NewCtxMap(".", " ", nil)
		baseContext.Set("service", "api")
		baseContext.Set("version", "1.0.0")
		baseContext.Set("env", "prod")

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			requestContext := baseContext.WithPairs(
				"request_id", fmt.Sprintf("req_%d", i),
				"user_id", i%1000,
				"endpoint", "/api/users",
				"method", "GET",
				"status", 200,
				"duration_ms", 150+i%100,
			)
			_ = requestContext.String()
		}
	})

	b.Run("nested_context", func(b *testing.B) {
		// Simulates nested contexts with prefixes (e.g., service.database.query)
		baseContext := createPopulatedMap(10)

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			dbContext := baseContext.WithPrefix("db")
			queryContext := dbContext.WithPrefix("query").WithPairs(
				"table", "users",
				"duration_ms", i%1000,
				"rows_affected", i%100,
			)
			_ = queryContext.String()
		}
	})

	b.Run("context_inheritance", func(b *testing.B) {
		// Simulates passing context through call chain with field additions
		rootContext := NewCtxMap(".", " ", nil)
		rootContext.Set("trace_id", "abc123")
		rootContext.Set("span_id", "def456")

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			// Service layer adds fields
			serviceContext := rootContext.WithPairs("service", "user-service")

			// Repository layer adds fields
			repoContext := serviceContext.WithPairs("repository", "user-repo", "query", "SELECT * FROM users")

			// Database layer adds fields
			dbContext := repoContext.WithPairs("db_host", "localhost", "db_name", "myapp")

			_ = dbContext.String()
		}
	})
}

// Edge Case Benchmarks

func BenchmarkEdgeCases(b *testing.B) {
	b.Run("empty_map_operations", func(b *testing.B) {
		m := NewCtxMap(".", " ", nil)

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			_ = m.String()
			_ = m.AsMap()
			_ = m.Len()
			_, _ = m.Get("non_existent")
		}
	})

	b.Run("large_values", func(b *testing.B) {
		largeValue := make([]byte, 1024) // 1KB value
		for i := range largeValue {
			largeValue[i] = byte(i % 256)
		}

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			m := NewCtxMap(".", " ", nil)
			m.Set("large_key", largeValue)
			_ = m.Clone()
		}
	})

	b.Run("deep_prefix_nesting", func(b *testing.B) {
		baseMap := createPopulatedMap(10)

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			current := baseMap
			// Create deeply nested prefixes
			for j := 0; j < 10; j++ {
				current = current.WithPrefix(fmt.Sprintf("level%d", j))
			}
			_ = current.String()
		}
	})
}

// Comparison Benchmarks (vs standard map)

func BenchmarkComparisonRead(b *testing.B) {
	b.Run("standard_map_read", func(b *testing.B) {
		m := make(map[string]any, 1000)
		for i := 0; i < 1000; i++ {
			m[fmt.Sprintf("key%d", i)] = i
		}

		var mu sync.RWMutex
		keys := generateKeys(1000, "key")

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			idx := i % 1000
			mu.RLock()
			_ = m[keys[idx]]
			mu.RUnlock()
		}
	})

	b.Run("ctxmap_read", func(b *testing.B) {
		m := createPopulatedMap(1000)
		keys := generateKeys(1000, "key")

		b.ResetTimer()
		b.ReportAllocs()

		for i := 0; i < b.N; i++ {
			idx := i % 1000
			_, _ = m.Get(keys[idx])
		}
	})
}

func BenchmarkComparisonItoa(b *testing.B) {
	val := 123456789
	b.Run("fmt_sprint_int", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = fmt.Sprint(val)
		}
	})
	b.Run("strconv_itoa", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = strconv.Itoa(val)
		}
	})
	b.Run("ctxmap_itoa", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = itoa64(int64(val))
		}
	})
}

func BenchmarkComparisonUint64(b *testing.B) {
	val := uint64(9876543210)
	b.Run("fmt_sprint_uint64", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = fmt.Sprint(val)
		}
	})
	b.Run("strconv_format_uint", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = strconv.FormatUint(val, 10)
		}
	})
	b.Run("ctxmap_utoa64", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = utoa64(val)
		}
	})
}
