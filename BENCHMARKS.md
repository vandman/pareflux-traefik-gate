<!-- FR: Définit les mesures reproductibles du runtime actuel. EN: Defines reproducible measurements for the current runtime. Called by: maintainers; Calls: integration fixture. -->
# Runtime performance verification

[Français](BENCHMARKS.FR.md)

There is no published capacity measurement for the current protection engine.
Passing native Go or real Traefik integration tests demonstrates behavior, not
throughput or resistance to a distributed flood. Native timings also do not
represent the Yaegi interpreter used by Traefik.

## Reproducible measurement

Use the isolated `test/docker-compose.yml` stack, which loads the local source
and disables CrowdSec online API communication. Do not generate attack traffic
against a production system or share synthetic alerts.

```bash
./scripts/ci-docker.sh compose --project-name pareflux-perf -f test/docker-compose.yml up --build --detach --wait
curl --header 'Host: pareflux.localhost' http://127.0.0.1:18080/
# Run measurements against this loopback-only fixture.
./scripts/ci-docker.sh compose --project-name pareflux-perf -f test/docker-compose.yml down --volumes
```

Use the same machine, backend, load generator, request count and concurrency for
comparisons. Record the source commit, Go/Traefik/CrowdSec/cache versions, CPU and
memory limits, warm-up, payload, mode, AppSec settings and number of decisions.
Measure allowed and blocked traffic separately, stream IP/range lookup, live
cache hits/misses, body inspection and snapshot synchronization. Repeat each
measurement and report throughput, p50/p95/p99 latency, errors and peak memory.
A local result is not a universal capacity claim. Repeat measurements when the
engine, runtime or dependencies change.
