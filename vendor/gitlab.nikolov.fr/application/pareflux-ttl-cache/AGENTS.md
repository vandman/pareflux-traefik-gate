# AGENTS — PareFlux TTL Cache

## Scope

These instructions apply to this repository.

## Rules

- License all original source under Apache-2.0.
- Do not add GPL-3.0, AGPL, unknown-license, or vendored dependencies.
- Keep the core cache free of background goroutines and persistent storage.
- Preserve compatibility semantics: zero TTL does not insert, negative TTL
  never expires, and positive TTL expires.
- Run `go test -race ./...` and benchmarks before release.
- Keep English and French documentation synchronized.
- Apply the workspace bilingual file/function comment standard to new code.
