[![Go](https://github.com/balinomad/go-ctxmap/actions/workflows/go.yml/badge.svg)](https://github.com/balinomad/go-ctxmap/actions/workflows/go.yml)

# go-ctxmap

*A thread-safe Go key-value map for contextual data. Features an immutable-style API and is heavily optimized for repeated string serialization via caching. Ideal for structured logging, request contexts, and dynamic configuration.*

It is perfect for use in:

  - **Structured Logging**: A base logger context is created at startup (`service`, `version`). Each HTTP request gets a child context with `WithPairs` (`request_id`, `user_id`). The final `String()` call is fast.
  - **Configuration Management**: A global configuration can be held in a `CtxMap`. Sub-components can create prefixed, immutable views (`dbConfig := globalConfig.WithPrefix("database")`). This prevents components from accidentally modifying shared configuration.
  - **Distributed Tracing Metadata**: Storing and propagating span tags or baggage items. As a request moves through services, new tags can be added immutably.
  - **HTTP Middleware Context**: A `CtxMap` can be created per request and passed via `context.Context`. Middleware can add information (e.g., authentication data, rate-limit status) in a thread-safe way, using prefixes to avoid key collisions.
  - **Feature Flag/Experimentation Context**: Store attributes about a user or request (`user_id`, `region`, `plan_type`). This context can be passed to a feature flag engine to make decisions. The `String()` method is useful for debugging which flags were evaluated.
  - **Building Dynamic Queries**: Assembling parameters for an API call or database query where different parts of the application contribute parameters. Prefixes can be used to group parameters (e.g., `filter`, `sort`).

## ✨ Features

  - **High-Performance Serialization**: A granular caching system dramatically speeds up repeated calls to the `String()` method. When only a few fields change between calls, only those fields are re-formatted, making it ideal for high-throughput structured logging.
  - **Thread-Safe by Design**: All operations are safe for concurrent use. It uses `sync.RWMutex` to allow for highly concurrent reads.
  - **Immutable-Style API**: Methods like `WithPairs`, `WithPrefix`, and `Merge` return new, independent `CtxMap` instances. This allows you to share a base context across goroutines while derived contexts can be modified without causing data races or requiring locks.
  - **Contextual Prefixes**: Easily namespace keys to avoid collisions when merging contexts from different application layers (e.g., `http.auth.user_id` vs. `db.query.user_id`).
  - **Fully Customizable Output**: You have full control over the output format. You can specify the separator for key prefixes (`.`, `:`, etc.), the separator between key-value pairs (`     `, `|`, etc.), and even provide a custom function to format each pair.
  - **Zero Dependencies**: A lightweight package that relies only on the Go standard library.


## 🚀 Usage

Here's a basic example demonstrating the core concepts of creating a base context and deriving a new one for a specific task.

```go
package main

import (
	"fmt"
	"log"

	"github.com/balinomad/go-ctxmap"
)

func main() {
	// 1. Create a base context for your application.
	// We'll use "." for key prefixes and " " to separate fields.
	// A nil stringer function uses the default "key=value" format.
	baseCtx := ctxmap.NewCtxMap(".", " ", nil)
	baseCtx.Set("service", "user-api")
	baseCtx.Set("version", "1.2.3")

	// 2. In a request handler, create a new map with request-specific data.
	// WithPairs is immutable; it returns a new map without modifying baseCtx.
	requestCtx := baseCtx.WithPairs(
		"request_id", "abc-123",
		"user_id", 42,
	)

	// 3. The String() method is called automatically by fmt functions.
	// It's heavily optimized and very fast on subsequent calls.
	log.Printf("handled request: %s", requestCtx)
	// Example output (order of keys is not guaranteed):
	// 2025/08/24 13:18:22 handled request: service=user-api version=1.2.3 request_id=abc-123 user_id=42
}
```

## 📌 Installation

```bash
go get github.com/balinomad/go-ctxmap@latest
```

## 📘 API Highlights

| Method | Description |
| :--- | :--- |
| `NewCtxMap(keySep string, fieldSep string, stringer func(k string, v any) string)` | Creates a new `CtxMap` with custom separators and formatting. |
| `Set(key string, value any)` | Sets a key-value pair. Safe for concurrent use. |
| `Get(key string) (any, bool)` | Retrieves a value by its raw key (prefix is not used). |
| `GetPrefixed(key string) (any, bool)` | Retrieves a value by its fully prefixed key (e.g., "prefix.key"). |
| `Delete(key string)` | Removes a key from the map. |
| `WithPairs(keyValues ...any)` | **(Immutable)** Returns a *new* map with additional key-value pairs. |
| `WithPrefix(prefix string)` | **(Immutable)** Returns a *new* map with a key prefix added. |
| `Merge(other *CtxMap)` | **(Immutable)** Returns a *new* map combining the receiver and another `CtxMap`. |
| `Clone() *CtxMap` | **(Immutable)** Returns a deep copy of the map. |
| `String() string` | Returns a cached, string representation of the map. Very fast on repeated calls. |
| `AsMap() map[string]any` | Returns the map's data as a `map[string]any`. **Warning**: May return the internal map for performance; do not modify. |
| `ToMapCopy() map[string]any` | Returns a *safe copy* of the map's data. |
| `Len() int` | Returns the number of items in the map. |
| `Range(fn func(k, v))` | Iterates over a snapshot of the map, applying prefixes to keys. |

-----

## 🔧 Advanced Example: Structured Logging Context

`CtxMap` is ideal for building up a structured logging context as a request flows through your application. Prefixes help organize data from different layers (middleware, services, database), and the final `String()` call is efficient.

```go
package main

import (
	"log"
	"os"
	"time"

	"github.com/balinomad/go-ctxmap"
)

// Simulates a middleware that processes a request.
func handleRequest(baseCtx *ctxmap.CtxMap, requestID int) {
	// 1. Create a request-specific context. This is cheap and doesn't lock the baseCtx.
	requestCtx := baseCtx.WithPairs(
		"request_id", requestID,
		"user_id", 12345,
	)
	log.Printf("[Request Start] %s", requestCtx)

	// 2. Perform a sub-operation, like a database call, with a prefixed context.
	// This helps organize keys and avoids collisions.
	dbCtx := requestCtx.WithPrefix("db")
	performDatabaseQuery(dbCtx)

	// 3. The original request context is unchanged by the prefixed operations.
	// We can add final timing information before logging.
	log.Printf("[Request End] %s", requestCtx.WithPairs("duration_ms", 50))
}

// Simulates a database operation that adds its own context.
func performDatabaseQuery(ctx *ctxmap.CtxMap) {
	// Add query-specific details.
	queryCtx := ctx.WithPairs(
		"query_hash", "a1b2c3d4",
		"table", "users",
	)
	time.Sleep(50 * time.Millisecond) // Simulate work
	log.Printf("[DB Query] %s", queryCtx)
}

func main() {
	// Create a base logger context at startup with static application info.
	appContext := ctxmap.NewCtxMap(".", " ", nil)
	appContext.Set("service", "worker-pool")
	appContext.Set("version", "1.0.1")
	appContext.Set("pid", os.Getpid())

	// Simulate handling multiple "requests" or jobs.
	for i := 1; i <= 3; i++ {
		handleRequest(appContext, i)
		time.Sleep(100 * time.Millisecond)
	}
}
```

## ⚡ Performance & Caching

The primary performance goal of `go-ctxmap` is to make the `String()` operation extremely fast, especially when a map is serialized repeatedly with minor changes.

This is achieved through a multi-level caching strategy:

1.  **Full String Caching**: If `String()` is called and no data has changed since the last call, the previously computed string is returned instantly without any new allocations or computations.
2.  **Granular Field Caching**: When a value is set via `Set()` or added via `WithPairs()`, the map marks only the affected keys as "dirty." When `String()` is called next:
  - The formatted strings for "clean" (unchanged) keys are retrieved from an internal cache.
  - Only the "dirty" keys are re-formatted.
  - The final string is built by joining the cached and newly formatted parts.

This means that if you have a context with 20 fields and you only change one, the cost of the next `String()` call is close to formatting a single field, not all 20.

## 🤝 Concurrency Model

`CtxMap` is designed for high-concurrency environments and guarantees safety through two primary mechanisms:

1.  **Internal Locking**: All methods that modify the map's internal state (like `Set`, `Delete`, `Clear`) use a `sync.RWMutex` to ensure that writes are serialized and that reads occurring during a write are not subject to race conditions. Reads (`Get`, `Len`, `String`) use a read lock, allowing multiple goroutines to read from the same map concurrently.
2.  **Immutability**: The methods `WithPairs`, `WithPrefix`, `Merge`, and `Clone` do not modify the original map. Instead, they return a new `CtxMap` instance with its own data and locks. This is a powerful pattern for concurrency: you can safely pass a `CtxMap` to multiple goroutines, and if they need to add context, they can create their own "local" version without ever needing to lock the original. This significantly reduces lock contention in highly parallel workflows.

## ⚖️ License

MIT License — see `LICENSE` file for details.