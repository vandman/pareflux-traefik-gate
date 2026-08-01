# Runtime benchmarks

## Traefik/Yaegi comparison

The first PareFlux Gate benchmark compared:

- Traefik `v3.7.5`;
- upstream plugin `v1.6.0` at commit
  `1f6a8991c8f9273cf21a5da3a6e6875a5c0034cc`;
- PareFlux Gate using PareFlux TTL Cache `v0.1.0`;
- a live-mode allowed-decision hot-cache path;
- 50,000 requests per run, concurrency 32, three runs;
- the same host, backend, mock LAPI, images, and load generator.

The upstream checkout and GPL cache remained outside this repository.

| Implementation | Requests/s | p50 | p95 | p99 | Errors |
| --- | ---: | ---: | ---: | ---: | ---: |
| Upstream cache | 3,427 | 7.966 ms | 20.271 ms | 28.663 ms | 0 |
| PareFlux cache | 3,365 | 8.132 ms | 20.516 ms | 29.452 ms | 0 |

The measured throughput difference was approximately `-1.8%`, within the
predeclared 5% native and 10% real-runtime tolerances. The benchmark is a
machine-local comparison, not a universal capacity claim.

## Native cache result

On the same host, the PareFlux cache hot `Get` path measured approximately
19–20 ns/op with zero allocation. The external upstream reference measured
approximately 24–25 ns/op with zero allocation. PareFlux `Set` measured
approximately 80 ns/op against approximately 82–90 ns/op upstream.

Benchmarks must be rerun for every Traefik, Go, Yaegi, or cache version change.
