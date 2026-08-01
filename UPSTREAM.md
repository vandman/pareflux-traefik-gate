# Upstream provenance

## Imported work

- Repository: `https://github.com/maxlerebourg/crowdsec-bouncer-traefik-plugin`
- Tag: `v1.6.0`
- Commit: `1f6a8991c8f9273cf21a5da3a6e6875a5c0034cc`
- License: Apache-2.0

The upstream checkout was made outside this product repository. Only selected
Apache-2.0 source and test files were copied into a newly initialized Git
repository.

The upstream `.git` directory, `vendor/`, `go.mod`, `go.sum`, examples, and the
GPL-3.0 `github.com/leprosus/golang-ttl-map` implementation were never copied
into this repository.

## Material modifications

- Module and package identity changed to PareFlux Gate.
- The GPL-3.0 TTL cache dependency was replaced by
  `gitlab.nikolov.fr/application/pareflux-ttl-cache`.
- Manifest, documentation, versioning, CI, and attribution were replaced.
- Future PareFlux changes are tracked in this repository.
- The Apache-2.0 `github.com/maxlerebourg/simpleredis` v1.0.12 client was
  hardened and moved into `pkg/simpleredis` so vendoring remains reproducible.
