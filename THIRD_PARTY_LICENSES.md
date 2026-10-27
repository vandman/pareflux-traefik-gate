# Third-party licenses

| Component | Version | License |
| --- | --- | --- |
| Crowdsec Bouncer Traefik Plugin derived source | v1.6.0 | Apache-2.0 |
| PareFlux TTL Cache | v0.1.3 | Apache-2.0 |
| simpleredis derived source | v1.0.12 | Apache-2.0 |

Vendored dependencies are limited to the reviewed versions recorded by
`go.mod`, `go.sum`, and `vendor/modules.txt`. Modified simpleredis source is
maintained in `pkg/simpleredis` so dependency vendoring remains reproducible.

The cache includes the hash-verified expiry-priority patch recorded in
`patches/ttl-cache-expiry.json`. See [Français](THIRD_PARTY_LICENSES.FR.md).
