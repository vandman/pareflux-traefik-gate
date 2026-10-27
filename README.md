# PareFlux Gate

**Connect Traefik to CrowdSec to block malicious access to your web applications.**

> **The plugin alone is not a complete protection system.** PareFlux Gate does
> **not include CrowdSec**. In the standard installation, a separate CrowdSec
> service must be available, either already installed or deployed alongside Traefik.
> **The PareFlux console is optional; the CrowdSec protection source is not.**

**English** · [Français](https://www.github.com/vandman/pareflux-traefik-gate/blob/v0.1.24/LISEZMOI.md) · [Traefik catalog](https://plugins.traefik.io/plugins/6a6da8539dbe8a37899c23c8/pare-flux-gate)

PareFlux Gate runs inside Traefik, the service that receives connections to your
web applications. CrowdSec runs separately: it detects suspicious activity and
provides blocking instructions, such as refusing an IP address for one hour.
PareFlux Gate applies those instructions to incoming connections.

CrowdSec can also examine the current web request to identify an attack. This
optional CrowdSec feature is called **AppSec**; it is not included in the plugin.
The **PareFlux console (PareFlux Control)** is a separate, optional interface for
viewing and administering the protection.

| Component | Needed? | Included in the plugin? |
| --- | --- | --- |
| Traefik, which receives connections to applications | Yes | No |
| PareFlux Gate, which applies protection inside Traefik | Yes | This is the plugin |
| CrowdSec, which supplies blocking instructions | Yes, as the protection source | **No; it operates separately** |
| AppSec, the CrowdSec feature that examines web requests | Optional | No |
| PareFlux console, for viewing and administering protection | Optional | No |

The complete Compose example installs Traefik with PareFlux Gate and a separate
CrowdSec service with AppSec enabled. **The example assembles these services;
CrowdSec is not embedded in the plugin.**

## Contents

- [Features](#features)
- [Choose a mode](#choose-a-mode)
- [Installation](#installation)
- [Verify protection](#verify-protection)
- [Settings and failure policies](#settings-and-failure-policies)
- [Troubleshooting](#troubleshooting)
- [Architecture](#architecture)

## Features

| Need | What Gate does |
| --- | --- |
| Apply blocking instructions | Refuse an IP address or present a captcha when instructed by CrowdSec |
| Examine web requests | Optionally ask CrowdSec AppSec to detect malicious requests before forwarding them |
| Avoid querying CrowdSec for every visitor | Keep synchronized blocking instructions in memory, or cache individual lookups |
| Identify visitors behind a proxy | Forwarded identity accepted only from an explicitly trusted proxy |
| Customize rejections | Configurable HTTP status, ban HTML, captcha provider and template |
| Diagnose blocking | Logs and an optional remediation marker in responses and internal request headers |

A **decision** is a blocking instruction with a scope (an IP or network range)
and an expiration. **LAPI** means CrowdSec’s local API: the service that Gate
calls to obtain those instructions, usually on port 8080. A **bouncer** is a
component that applies them; Gate is a bouncer. A **TTL** is the lifetime of a
temporary cache entry. Redis is an optional external cache service.

CrowdSec analyzes logs and produces decisions. Gate enforces them; it does not
install scenarios or replace the CrowdSec engine. Persistent decisions stay in
LAPI; restarting Traefik clears the local cache. The optional community feed contains blocking instructions supplied by
CrowdSec for threats observed elsewhere. Access depends on CrowdSec registration
and the feeds enabled for that account; it is not required for local detection.

## Choose a mode

| Mode | Decision source | Cache | When to use it |
| --- | --- | --- | --- |
| `stream` | CrowdSec LAPI, full snapshot every 60 seconds by default | Separate in-memory IP/range store; no active-ban eviction | Recommended starting point for LAPI decisions with low per-request overhead |
| `live` | LAPI on cache misses | Local or Redis; results cached for up to 60 seconds by default | Query individual IPs as they arrive |
| `none` | LAPI on every request | No IP decision lookup cache | Require a fresh LAPI answer for every request; this does **not** disable protection |
| `appsec` | CrowdSec AppSec only | No IP decision lookup | Use HTTP inspection when another component already enforces IP decisions; set `crowdsecAppsecEnabled: true` |
| `alone` | CrowdSec Central API, every two hours | Same separate IP/range store as stream | Consume the community feed with valid CAPI credentials; does not include your local LAPI decisions |

AppSec can complement IP decision modes by setting
`crowdsecAppsecEnabled: true`. The default mode is `live`; the example below
explicitly selects `stream`.

## Installation

The public example pins **v0.1.24**. Static configuration loads the plugin at
startup; dynamic configuration enables it and attaches it to protected routes.
Registering the plugin alone does not protect any route.

### 1. Prepare CrowdSec

Gate connects **directly to CrowdSec**, without going through the PareFlux
console. Use CrowdSec’s **local API address**, not its website or the console URL.
For example, `crowdsecLapiHost: crowdsec:8080` means that Traefik can resolve the
service name `crowdsec` and reach its port 8080 on a shared private network.
For a remote engine, use its reachable private hostname and configure HTTPS.
Do not expose this API publicly.

On the CrowdSec host, create a dedicated key:

```bash
sudo cscli bouncers add pareflux-traefik
# With Docker instead: docker compose exec crowdsec cscli bouncers add pareflux-traefik
```

The command prints a secret API key. Save it securely and mount that key read-only
**inside the Traefik container** at `/run/secrets/crowdsec-bouncer.key`.
The user running Traefik must be able to read it. For an existing Compose
service, save only the displayed key in `./secrets/crowdsec-bouncer.key`
(outside Git) and add this mount:
```yaml
services:
  traefik:
    volumes:
      - ./secrets/crowdsec-bouncer.key:/run/secrets/crowdsec-bouncer.key:ro
```


For AppSec, CrowdSec also needs an AppSec component listening on `:7422`, its
acquisitions and rules. Otherwise start with `crowdsecAppsecEnabled: false`.
Behavior detection from logs additionally requires a separate Traefik log
acquisition in CrowdSec.

### 2. Register the plugin in Traefik

In the **static** `traefik.yml` file:

```yaml
experimental:
  plugins:
    parefluxgate:
      moduleName: github.com/vandman/pareflux-traefik-gate
      version: v0.1.24
```

Restart Traefik after changing the plugin version. Use the existing file
provider, or register the dynamic file below:

```yaml
providers:
  file:
    filename: /etc/traefik/dynamic/pareflux.yml
```

### 3. Create and attach the middleware

In `/etc/traefik/dynamic/pareflux.yml`, replacing the domain, service and
CrowdSec addresses with your installation values:

```yaml
http:
  routers:
    app:
      rule: Host(`app.example.com`)
      entryPoints: [websecure]
      tls: {}
      service: app
      middlewares: [pareflux-gate]
  services:
    app:
      loadBalancer:
        servers:
          - url: http://app:80
  middlewares:
    pareflux-gate:
      plugin:
        parefluxgate:
          enabled: true
          crowdsecMode: stream
          crowdsecLapiHost: crowdsec:8080
          crowdsecLapiKeyFile: /run/secrets/crowdsec-bouncer.key
          crowdsecAppsecEnabled: false
          remediationHeadersCustomName: X-Pareflux-Remediation
```

This example assumes an existing `websecure` entrypoint and certificates managed
by your usual Traefik TLS configuration. Keep existing SSO and other security
middlewares. A Docker router references this file-defined middleware as
`pareflux-gate@file`.

### Alternative: Docker labels

When both router and middleware are defined by the Docker provider, use this
alternative **instead of** the file-defined middleware:

```yaml
services:
  app:
    labels:
      traefik.http.routers.app.middlewares: pareflux-gate
      traefik.http.middlewares.pareflux-gate.plugin.parefluxgate.enabled: "true"
      traefik.http.middlewares.pareflux-gate.plugin.parefluxgate.crowdsecMode: stream
      traefik.http.middlewares.pareflux-gate.plugin.parefluxgate.crowdsecLapiHost: crowdsec:8080
      traefik.http.middlewares.pareflux-gate.plugin.parefluxgate.crowdsecLapiKeyFile: /run/secrets/crowdsec-bouncer.key
      traefik.http.middlewares.pareflux-gate.plugin.parefluxgate.crowdsecAppsecEnabled: "false"
```

Add the usual router, entrypoint and service labels. The key remains a file
mounted in Traefik, never a secret in labels. Attach the same middleware to
other application routers to protect them too.

### Optional: examine requests with AppSec

First make IP blocking work. Then enable CrowdSec’s separate AppSec listener
on port 7422 with its acquisitions and rule collections. The
[AppSec acquisition example](https://www.github.com/vandman/pareflux-traefik-gate/blob/v0.1.24/examples/compose/appsec.yaml) and the Compose
`COLLECTIONS` setting show this setup. See also the
[CrowdSec AppSec guide](https://docs.crowdsec.net/docs/appsec/quickstart/traefik/).
Do not copy another plugin’s module name or version from that guide.

Add these options to the **existing protection middleware**:

```yaml
crowdsecAppsecEnabled: true
crowdsecAppsecHost: crowdsec:7422
crowdsecAppsecBodyLimit: 1048576
```

The AppSec key inherits the LAPI key in LAPI modes unless overridden. AppSec
examines metadata and the body; cookies and `Authorization` are deliberately
excluded. By default a body over **1 MiB is rejected with HTTP 413**. Configure
[explicit upload exceptions](https://www.github.com/vandman/pareflux-traefik-gate/blob/v0.1.24/docs/security-runtime.EN.md#appsec-inspection)
before enabling it on applications accepting large uploads. The inspection
budget is 3 seconds; keep a finite Traefik entrypoint `readTimeout`, for example
60 seconds, aligned with `crowdsecAppsecReadTimeoutSeconds`.

### Complete demonstration

The [Compose example](https://www.github.com/vandman/pareflux-traefik-gate/tree/v0.1.24/examples/compose)
includes Traefik, CrowdSec, AppSec and Whoami. Obtain the public repository
once, then start the demonstration:

```bash
git clone https://github.com/vandman/pareflux-traefik-gate.git
cd pareflux-traefik-gate/examples/compose
docker compose up --detach --wait
curl --header 'Host: pareflux.localhost' http://127.0.0.1:18080/
# Remove only this demonstration stack and its data:
docker compose down --volumes
```

The demonstration explicitly enables AppSec and disables CrowdSec online API
communication (`DISABLE_ONLINE_API: "true"`), so test traffic is not shared. It
does not configure Traefik access-log acquisition: automatic detection from
logs needs that additional setup. Its one-second synchronization interval is
for demonstration; use the 60-second default as a starting point in production.

The example listens on loopback only. Its embedded key is disposable and is
not intended for production. Use a dedicated key in production and prefer a
Docker socket proxy to mounting the socket directly.

## Verify protection

1. Confirm that a normal request reaches the application.
2. On a test installation, add a temporary decision for the IP **actually
   resolved by Gate**, then repeat the request from that IP.
3. Allow for the `stream` synchronization interval; verify rejection, remove
   the decision, then confirm normal access again.

```bash
# On the TEST CrowdSec engine; this is a documentation-only IP:
cscli decisions add --ip 192.0.2.123 --duration 2m --reason pareflux-test
cscli decisions delete --ip 192.0.2.123
```

Replace `192.0.2.123` with the test client's IP. Do not ban the IP used to
administer the installation. A successful IP decision test alone does not
prove that log acquisition or AppSec rules work.

## Settings and failure policies

| Option | Default | Purpose |
| --- | --- | --- |
| `enabled` | `false` | Explicitly enable the protection middleware |
| `crowdsecMode` | `live` | Select decision source and update behavior |
| `crowdsecLapiKeyFile` | Empty | Read the LAPI key from a mounted secret |
| `updateIntervalSeconds` | `60` | LAPI stream refresh interval |
| `defaultDecisionSeconds` | `60` | Maximum cached result lifetime in `live` mode |
| `crowdsecAppsecEnabled` | `false` | Enable HTTP inspection |
| `remediationStatusCode` | `403` | HTTP status returned for a ban |
| `forwardedHeadersTrustedIps` | `[]` | Proxies allowed to supply the visitor's IP |
| `clientTrustedIps` | `[]` | Clients bypassing **all** Gate checks |

The [complete configuration reference](https://www.github.com/vandman/pareflux-traefik-gate/blob/v0.1.24/docs/configuration.EN.md)
covers types, defaults, secret files, TLS, Redis and captcha. Names match the
PareFlux code: an option present in another CrowdSec fork is not necessarily
available here.

| Failure | Default policy | Setting |
| --- | --- | --- |
| LAPI/CAPI stream marked unhealthy | After failure threshold and grace, refuse IPs without a known decision; unexpired bans remain enforced | `crowdsecStreamUnreachableBlock: true` |
| AppSec connection establishment failure | Refuse after qualifying outage grace | `crowdsecAppsecUnreachableBlock: true` |
| AppSec HTTP 502/503/504 | Refuse after qualifying outage grace | `crowdsecAppsecFailureBlock: true` |
| Required Redis lookup unavailable | Refuse the request needing that lookup | `redisCacheUnreachableBlock: true` |

Setting one of these options to `false` prioritizes availability for that
specific failure. `live` and `none` also reject requests whose LAPI query fails.
The stream allows 3 consecutive update failures and a 60-second grace by
default, after a successful synchronization. Startup without a successful
synchronization is blocked. AppSec grace covers only connection establishment
failures and HTTP 502/503/504 after a successful inspection; inspection timeouts
and HTTP 500 remain blocking. Exceeding `streamDecisionCapacity` immediately
refuses unknown clients. See [precise thresholds and limits](https://www.github.com/vandman/pareflux-traefik-gate/blob/v0.1.24/docs/security-runtime.EN.md).

Trusted proxies and exempt clients serve different purposes:
`forwardedHeadersTrustedIps` allows resolving the visitor IP, while
`clientTrustedIps` bypasses protection for the resolved IP. Do not indiscriminately
trust public networks. A NAT router does not supply HTTP identity headers.
Routes with the same resolved CrowdSec configuration share an engine; changing
its address, credentials or engine options creates a separate engine.

## Troubleshooting

| Symptom | Check |
| --- | --- |
| Requests are never filtered | `enabled: true`, middleware attached to the correct router and provider (`@file` / `@docker`) |
| Wrong IP or all visitors blocked | Network peer, proxy chain and `forwardedHeadersTrustedIps`; do not trust arbitrary forwarded headers |
| Rejections after a CrowdSec outage | Gate logs, LAPI/AppSec connectivity and the failure policies above |
| LAPI `401` / `403` | Bouncer key, file readable inside Traefik and matching CrowdSec engine |
| A ban is not immediately applied | Selected mode, stream delay or `live` cache, actual resolved IP |
| LAPI works but AppSec rejects everything | `:7422` service, AppSec acquisitions and rules, key and AppSec logs |
| Rejections appear as new attacks in logs | Retain the remediation marker in access logs and configure its exclusion in CrowdSec processing |

## Architecture

Diagrams and detailed explanations are in the separate
[PareFlux Gate architecture](https://www.github.com/vandman/pareflux-traefik-gate/blob/v0.1.24/docs/architecture.EN.md)
document: components, network traffic, HTTP processing and failure decisions.

## PareFlux console and Traefik dashboard

The protection plugin works without PareFlux Control. The optional dashboard
integration adds a PareFlux link, a “Connected / Protected” tile and private
URL exchange with Control. See the [dedicated configuration](https://www.github.com/vandman/pareflux-traefik-gate/blob/v0.1.24/docs/dashboard-menu.EN.md).
URLs come from deployment configuration; standalone Control displays no ingress
return link.

## Storage choices: local, Redis, PostgreSQL, or MariaDB

PostgreSQL and MariaDB are supported, but they are configured on CrowdSec LAPI,
not on the PareFlux middleware. This separation keeps each storage role clear:

| Storage | Used by | Purpose |
| --- | --- | --- |
| In-memory decision store | PareFlux Gate | Active stream/alone IP and range decisions, without eviction |
| In-process TTL cache | PareFlux Gate | Temporary live lookup and captcha results for one engine |
| Redis | PareFlux Gate | Optional shared live/captcha cache for matching configurations; does not store stream bans |
| SQLite | CrowdSec LAPI | Default persistent storage for a simple single-node installation |
| PostgreSQL or MariaDB | CrowdSec LAPI | Persistent LAPI storage suitable for larger or replicated installations |

Changing the CrowdSec database does not change the PareFlux labels: Gate still
connects to `crowdsec:8080`, and CrowdSec owns the database connection.

The commands below merge two Compose files. `docker-compose.yml` provides
Traefik, Whoami, CrowdSec, and AppSec; the explicitly named `compose.override.*`
file adds the selected database and configures CrowdSec to use it.

For a shared Redis cache, use the Redis overlay. It selects `live` so individual
lookups actually use the cache; adding Redis does not replace the stream store.
The demo password file is disposable. Redis has no host-published port.

```bash
cd examples/compose
docker compose -f docker-compose.yml -f compose.override.redis.yml up --detach --wait
curl --header 'Host: pareflux.localhost' http://127.0.0.1:18080/
docker compose -f docker-compose.yml -f compose.override.redis.yml down --volumes
```

Redis is independent of Traefik, but this overlay configures its use by Gate.
This client uses plain Redis TCP; keep it on a trusted private network.

Run the complete example with PostgreSQL:

```bash
cd examples/compose
docker compose \
  -f docker-compose.yml \
  -f compose.override.postgresql.yml \
  up --detach --wait
curl --header 'Host: pareflux.localhost' http://127.0.0.1:18080/
docker compose \
  -f docker-compose.yml \
  -f compose.override.postgresql.yml \
  down --volumes
```

Or run the same stack with MariaDB:

```bash
cd examples/compose
docker compose \
  -f docker-compose.yml \
  -f compose.override.mariadb.yml \
  up --detach --wait
curl --header 'Host: pareflux.localhost' http://127.0.0.1:18080/
docker compose \
  -f docker-compose.yml \
  -f compose.override.mariadb.yml \
  down --volumes
```

The database passwords embedded in these examples are disposable defaults.
Use Docker secrets or another secret manager, TLS to an external database, and
durable backups in production. Switching an existing CrowdSec installation to
a new database does not migrate its existing machines, remediation keys,
alerts, or decisions automatically.

## Verification

```bash
go test -race ./...
go mod vendor
./scripts/patch-ttl-cache-vendor.sh
```

Every release must also load successfully as a local plugin in the pinned
Traefik image and pass the request, blocking and reload integration tests.
See the [measurement procedure](https://www.github.com/vandman/pareflux-traefik-gate/blob/v0.1.24/BENCHMARKS.md) for performance validation.

## Repositories and public releases

PareFlux is maintained by a French author who wants complete, first-class
documentation in French. A complete English version is maintained in parallel
because English is the de facto international language for technical
collaboration and the Traefik ecosystem.

The [GitLab repository](https://gitlab.nikolov.fr/application/pareflux-traefik-gate)
is the primary development repository and contains intermediate commits and
development releases. The [GitHub repository](https://www.github.com/vandman/pareflux-traefik-gate)
is the public distribution and contribution channel because the Traefik Plugin
Catalog only accepts public GitHub repositories.

Only stable snapshots are promoted to GitHub. A protected GitLab tag named
`publish/vX.Y.Z` creates one public commit and the immutable `vX.Y.Z` GitHub
tag. Public issues and pull requests are welcome; see
[CONTRIBUTING.md](https://www.github.com/vandman/pareflux-traefik-gate/blob/v0.1.24/CONTRIBUTING.md) for their integration path.

The [catalog publication guide](https://www.github.com/vandman/pareflux-traefik-gate/blob/v0.1.24/docs/publication.EN.md) describes automatic
README link preparation and release checks.

## License

Derived from the Apache-2.0 upstream v1.6.0; original attribution and changes are recorded in [UPSTREAM.md](https://www.github.com/vandman/pareflux-traefik-gate/blob/v0.1.24/UPSTREAM.md). PareFlux uses its own Apache-2.0 TTL cache.

Apache-2.0. GPL-3.0, AGPL, and unknown-license dependencies are rejected.

## CI trust contract

Branches and merge requests run without permanent publication secrets. A source
branch ending in `-nr` has no pipeline. Tests requiring Docker use `vm-runner`
and its disposable VM daemon; other checks use the container runner.
Only protected tags may receive publication credentials. Tags are created from
the operator workstation, with GitLab repository writes disabled for job tokens.
Deployments, identity administration and runner host maintenance run from the
workstation. No production SSH key, kubeconfig or OpenBao identity is provided
to these jobs.

[Protection runtime, limits and failure policies](https://www.github.com/vandman/pareflux-traefik-gate/blob/v0.1.24/docs/security-runtime.EN.md).
