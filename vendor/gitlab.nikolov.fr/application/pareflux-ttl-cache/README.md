# PareFlux TTL Cache

PareFlux TTL Cache is a small Apache-2.0 in-memory cache for security
decisions and Traefik middleware workloads.

## Contract

- Keys are strings and values are arbitrary Go values.
- A positive TTL is expressed in seconds.
- A zero TTL does not mutate the cache.
- A negative TTL never expires.
- Expired entries are removed lazily or through bounded explicit cleanup.
- `New` bounds the cache to 65,536 entries and evicts the oldest
  inserted or renewed entry in constant time; `NewWithCapacity` selects a
  smaller or larger explicit bound.
- The cache starts no background goroutine and performs no persistence.

## Usage

```go
cache := ttlcache.New()
cache.Set("198.51.100.10", "ban", 60)
value, found := cache.Get("198.51.100.10")
```

## Verification

```bash
go test -race ./...
go test -bench=. -benchmem -count=10 -cpu=1,4,16 ./...
```

Native benchmarks are necessary but not sufficient. Every release consumed by
PareFlux Gate is also benchmarked inside the matching Traefik/Yaegi runtime.

## License policy

The module is Apache-2.0 and intentionally has no runtime dependency. GPL-3.0,
AGPL, and unknown-license dependencies are rejected.
