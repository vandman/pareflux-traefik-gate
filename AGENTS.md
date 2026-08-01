# AGENTS — PareFlux Traefik Gate

## Scope

These instructions apply to this repository.

## Rules

- License original and derived source under Apache-2.0.
- Preserve upstream attribution and modified-file notices.
- Never add GPL-3.0, AGPL, unknown-license, or unreviewed vendored code.
- Keep the local plugin read-only at runtime.
- Keep persistent decisions in CrowdSec LAPI, not in the plugin cache.
- Treat the network peer address separately from forwarded identity headers.
- Verify every release with native Go tests and real Traefik/Yaegi tests.
- Keep English and French documentation synchronized.
- Apply the workspace bilingual file/function comment standard to new code.
