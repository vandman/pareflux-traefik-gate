<!-- FR: Explique les limites et preuves du moteur. EN: Explains engine limits and evidence. Called by: operators; Calls: configuration reference. -->
# Protection runtime and failure policies

[Français](security-runtime.FR.md) · [Configuration](configuration.EN.md)

## Engine lifecycle and decisions

Gate needs CrowdSec; Control is the optional console. Identical resolved upstream
configurations share one engine, synchronization workers and cache. Different
keys or certificates create separate engines. Releasing the last Traefik
configuration context cancels workers and outstanding HTTP calls.

Stream engines perform an initial synchronization before reporting healthy.
Every poll requests a full snapshot, avoiding lost incremental changes during
reloads or between consumers. The default interval is 60 seconds; shorter
intervals increase LAPI and Traefik/Yaegi work. `alone` polls the Central API
every two hours and uses the same decision store.

Active decisions are indexed by ID and IPv4/IPv6 prefix. Ban takes precedence
over captcha, including overlapping ranges. Removing one ID does not remove
another applicable decision. Malformed addresses, durations and unsupported
scopes are skipped and counted in logs and `skipped_decisions`.

The store never evicts active bans. `streamDecisionCapacity` defaults to 262144
(maximum 1000000). Overflow preserves known bans and **immediately refuses unknown
clients**, with an explicit error log, until a complete snapshot fits. Increasing
feed subscriptions therefore requires monitoring counts and sizing capacity.
There is no allow-unknown-on-overflow option.

Snapshots are decoded entry by entry; truncation, duplicate root keys or excessive
values never replace active decisions. JSON operations are limited to 64 KiB plus
native read-ahead buffering. Response size is limited to 512 bytes per configured
slot plus 1 MiB, with a 16 MiB minimum and 256 MiB maximum. An unusually large
response remains a synchronization error even if its decision count fits.

## Upstream failures

A refusal during an upstream failure protects traffic when Gate cannot verify
it; it does not necessarily mean that Gate itself crashed.

| Situation | Default behavior |
| --- | --- |
| Stream startup has never synchronized successfully | Refuse unknown clients |
| Stream outage after successful synchronization | Keep unexpired bans; refuse unknown clients only after **both** more than 3 consecutive failures and more than 60 seconds since last success |
| `updateMaxFailure: -1` | Disable the failure-count transition; the age grace alone does not make a previously healthy stream fail closed |
| AppSec connection establishment failure or HTTP 502/503/504 | After a successful inspection, allow grace while elapsed time is at most 60 seconds **or** the failure count is at most 3; after both are exceeded, refuse |
| AppSec inspection timeout, HTTP 500 or security refusal | No outage grace; refuse |
| Live/none LAPI query failure | Refuse |

`failureGraceSeconds` and `updateMaxFailure` change those defaults. Setting
`updateMaxFailure: -1` also leaves qualifying AppSec errors within grace after
an initial success. Setting `crowdsecStreamUnreachableBlock: false` allows
unknown clients during stream outages. `crowdsecAppsecUnreachableBlock: false`
permits connection establishment failures; `crowdsecAppsecFailureBlock: false`
permits HTTP 502/503/504. Neither permits inspection timeouts, HTTP 500 or attacks.
Do not confuse the configurable ban status with technical 408/413/503 responses.

## AppSec inspection

| Limit | Default / bound |
| --- | --- |
| Body prefix inspected | 1 MiB; configurable up to 16 MiB |
| Oversized body | HTTP 413 unless partial inspection is explicitly configured |
| Body reading plus inspection budget | 3 seconds; client cancellation propagates |
| Shared buffer memory | 64 MiB per Traefik process |
| Concurrent body readers | 1024 per Traefik process |
| Buffered bodies per client | 4 per IPv4 address or IPv6 /64; IPv6 /48 quota is four times that value |
| Client quota records | At most 4096; idle entries are removed |
| Post-inspection read deadline | 60 seconds from Gate entry; align with Traefik `respondingTimeouts.readTimeout` |

Bodyless requests still receive metadata inspection without consuming the body
quota. gRPC or WebSocket MIME/Upgrade headers do not exempt a body. Cookies and
`Authorization` are deliberately not forwarded: rules inspecting their contents,
including cookie-specific CRS rules, cannot inspect those fields.

One replay buffer uses progressively reserved 8 KiB chunks, up to the body limit
plus one detection byte. It is released after both AppSec and backend replay
writers finish consuming it, without waiting for the backend response. Four
parallel bodies share a quota behind an IPv4 NAT/CGNAT; adjust
`crowdsecAppsecMaxConcurrentPerClient` for legitimate parallel uploads while
considering the global limits.

Inspection expiry returns from the handler, but a wrapped reader that cannot
be interrupted retains its slot and quota until it actually finishes. Configure
a **finite Traefik entrypoint read timeout**, for example 60 seconds. Gate
restores a finite deadline where supported, never a zero deadline; setting
`crowdsecAppsecReadTimeoutSeconds: 0` leaves Traefik's deadlines untouched.

Reader admission and buffer bytes are separate bounds. Distributed clients can
still exhaust all readers (256 IPv4 addresses or 64 IPv6 /48 allocations at
four bodies per client), potentially until the entrypoint timeout. This limits
resource use; it is not a guarantee of availability against distributed floods.

Keep rejection on sensitive routes. Declare partial inspection only for precise
upload hosts, anchored paths and methods, for example:

```yaml
crowdsecAppsecBodyExceptions:
  - host: registry.example.org
    pathRegexp: '^/v2/.*/blobs/uploads(?:/.*)?$'
    methods: [POST, PATCH, PUT]
```

Up to 32 exceptions are accepted. They retain IP checks, metadata and prefix
inspection; remaining bytes are **not inspected**. Partial responses carry
`X-Pareflux-Appsec-Inspection: partial` and do not confirm full inspection in
dashboard status. `crowdsecAppsecBodyLimitAction: partial` applies to the whole
middleware, so reserve it for dedicated upload routes. A zero body limit inspects
metadata only. `crowdsecAppsecBodyExcludedContentTypes` defaults to `[]` and is
ignored; it cannot bypass inspection. Identical entrypoint/router protection
policies are executed once.

## Cache and administration

TTL/Redis accelerates live lookups and captcha results only. Configurations use
separate versioned Redis namespaces; Redis eviction cannot remove stream bans.
The local TTL cache examines up to 64 positions for expired entries before
evicting its oldest entry. Beyond that window, a valid cached lookup or captcha
result may be evicted; active stream bans remain in their separate store.

Before enforcing outage refusals, configure a narrowly scoped, verified
`clientTrustedIps` management address if needed. It bypasses **all** checks,
including AppSec. `forwardedHeadersTrustedIps` only permits a real upstream HTTP
proxy to provide visitor identity; it does not exempt its visitors. A NAT router
is not such a proxy. Never trust arbitrary public networks.

## Dashboard evidence and discovery

The dashboard separates Control connectivity, decision synchronization and this
request's AppSec inspection. An old healthy engine cannot confirm a new one.
Signed `range_decisions` refers only to the matching private menu engine: active
references, fresh synchronization, no failures and no overflow. Copy the actual
protection middleware's engine options into navigation instances.

Discovery requires a shared key of at least 32 bytes and a private registration
route restricted to Control. Requests and responses are HMAC signed, timestamped
and nonce-bound; replayed and expired announcements are rejected. Links expire
after 90 seconds without renewal even if an old health probe succeeded. No user
cookies or SSO credentials are exchanged. See the
[complete dashboard setup](dashboard-menu.EN.md).
