# PareFlux Gate

**Connect Traefik to CrowdSec to block malicious access to your web applications.**

> **The plugin alone is not a complete protection system.** PareFlux Gate does
> **not include CrowdSec**. In the standard installation, a separate CrowdSec
> service must be available, either already installed or deployed alongside Traefik.
> **The PareFlux console is optional; the CrowdSec protection source is not.**

**English** · [Français](https://www.github.com/vandman/pareflux-traefik-gate/blob/v0.1.17/LISEZMOI.md) · [Traefik catalog](https://plugins.traefik.io/plugins/6a6da8539dbe8a37899c23c8/pare-flux-gate)

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
| Enforce CrowdSec decisions | Bans and captcha remediations from LAPI, or community decisions in `alone` mode |
| Stop HTTP attacks | CrowdSec AppSec inspection, enabled separately from IP decisions |
| Reduce LAPI requests | `stream` and `live` modes, local TTL or Redis caching |
| Identify visitors behind a proxy | Forwarded identity accepted only from an explicitly trusted proxy |
| Customize rejections | Configurable HTTP status, ban HTML, captcha provider and template |
| Diagnose blocking | Logs and an optional remediation marker in responses and internal request headers |

CrowdSec analyzes logs and produces decisions. Gate enforces them; it does not
install scenarios or replace the CrowdSec engine. Persistent decisions stay in
LAPI; restarting Traefik clears the local cache. Community feed access depends
on CrowdSec registration and entitlements.

## Choose a mode

| Mode | Decision source | Cache | When to use it |
| --- | --- | --- | --- |
| `stream` | CrowdSec LAPI, refreshed every 60 seconds by default | Local or Redis | Recommended starting point for LAPI decisions with low per-request overhead |
| `live` | LAPI on cache misses | Local or Redis; results cached for up to 60 seconds by default | Query individual IPs as they arrive |
| `none` | LAPI on every request | No IP decision lookup cache | Require a fresh LAPI answer for every request; this does **not** disable protection |
| `appsec` | CrowdSec AppSec only | No IP decision lookup | Use HTTP inspection when another component already enforces IP decisions; set `crowdsecAppsecEnabled: true` |
| `alone` | CrowdSec Central API, every two hours | Local or Redis | Consume the community feed with valid CAPI credentials; does not include your local LAPI decisions |

AppSec can complement IP decision modes by setting
`crowdsecAppsecEnabled: true`. The default mode is `live`; the example below
explicitly selects `stream`.

## Installation

The public example pins **v0.1.16**. Static configuration loads the plugin at
startup; dynamic configuration enables it and attaches it to protected routes.
Registering the plugin alone does not protect any route.

### 1. Prepare CrowdSec

Provide a LAPI endpoint reachable from Traefik and a dedicated bouncer API key,
created with `cscli bouncers add pareflux-traefik`. Mount that key read-only
**inside the Traefik container** at `/run/secrets/crowdsec-bouncer.key`.
The user running Traefik must be able to read it.

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
      version: v0.1.16
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
          crowdsecAppsecEnabled: true
          crowdsecAppsecHost: crowdsec:7422
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
      traefik.http.middlewares.pareflux-gate.plugin.parefluxgate.crowdsecAppsecEnabled: "true"
      traefik.http.middlewares.pareflux-gate.plugin.parefluxgate.crowdsecAppsecHost: crowdsec:7422
```

Add the usual router, entrypoint and service labels. The key remains a file
mounted in Traefik, never a secret in labels. Attach the same middleware to
other application routers to protect them too.

### Complete demonstration

The [Compose example](https://www.github.com/vandman/pareflux-traefik-gate/tree/v0.1.17/examples/compose)
includes Traefik, CrowdSec, AppSec and Whoami. After obtaining the repository:

```bash
cd examples/compose
docker compose up --detach --wait
curl --header 'Host: pareflux.localhost' http://127.0.0.1:18080/
# Remove only this demonstration stack and its data:
docker compose down --volumes
```

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

The [complete configuration reference](https://www.github.com/vandman/pareflux-traefik-gate/blob/v0.1.17/docs/configuration.EN.md)
covers types, defaults, secret files, TLS, Redis and captcha. Names match the
PareFlux code: an option present in another CrowdSec fork is not necessarily
available here.

| Failure | Default policy | Setting |
| --- | --- | --- |
| LAPI/CAPI stream marked unhealthy | Block IPs missing from cache; cached bans remain enforced | `crowdsecStreamUnreachableBlock: true` |
| AppSec unreachable | Block the request | `crowdsecAppsecUnreachableBlock: true` |
| AppSec internal HTTP error | Block the request | `crowdsecAppsecFailureBlock: true` |
| Redis unreachable when enabled | Block the request | `redisCacheUnreachableBlock: true` |

Setting one of these options to `false` prioritizes availability for that
specific failure. `live` and `none` also reject requests whose LAPI query fails.
The `updateMaxFailure` counter controls stream health; it does not replace
these failure policies.

Trusted proxies and exempt clients serve different purposes:
`forwardedHeadersTrustedIps` allows resolving the visitor IP, while
`clientTrustedIps` bypasses protection for the resolved IP. Do not indiscriminately
trust public networks. Cache and synchronization workers are shared within the
Traefik process; separate middleware instances are not isolated CrowdSec engines.

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
[PareFlux Gate architecture](https://www.github.com/vandman/pareflux-traefik-gate/blob/v0.1.17/docs/architecture.EN.md)
document: components, network traffic, HTTP processing and failure decisions.

## PareFlux console and Traefik dashboard

The protection plugin works without PareFlux Control. The optional dashboard
integration adds a PareFlux link, a “Connected / Protected” tile and private
URL exchange with Control. See the [dedicated configuration](https://www.github.com/vandman/pareflux-traefik-gate/blob/v0.1.17/docs/dashboard-menu.EN.md).
URLs come from deployment configuration; standalone Control displays no ingress
return link.

## Storage choices: local, Redis, PostgreSQL, or MariaDB

PostgreSQL and MariaDB are supported, but they are configured on CrowdSec LAPI,
not on the PareFlux middleware. This separation keeps each storage role clear:

| Storage | Used by | Purpose |
| --- | --- | --- |
| In-process TTL cache | PareFlux Gate | Default ephemeral decision cache for one Traefik process |
| Redis | PareFlux Gate | Optional shared ephemeral cache for several Traefik processes |
| SQLite | CrowdSec LAPI | Default persistent storage for a simple single-node installation |
| PostgreSQL or MariaDB | CrowdSec LAPI | Persistent LAPI storage suitable for larger or replicated installations |

Changing the CrowdSec database does not change the PareFlux labels: Gate still
connects to `crowdsec:8080`, and CrowdSec owns the database connection.

The commands below merge two Compose files. `docker-compose.yml` provides
Traefik, Whoami, CrowdSec, and AppSec; the explicitly named `compose.override.*`
file adds the selected database and configures CrowdSec to use it.

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
go test -bench=. -benchmem ./...
go mod vendor
```

Every release must also load successfully as a local plugin in the pinned
Traefik image and pass end-to-end request benchmarks.

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
[CONTRIBUTING.md](https://www.github.com/vandman/pareflux-traefik-gate/blob/v0.1.17/CONTRIBUTING.md) for their integration path.

The [catalog publication guide](https://www.github.com/vandman/pareflux-traefik-gate/blob/v0.1.17/docs/publication.EN.md) describes automatic
README link preparation and release checks.

## License

Derived from the Apache-2.0 upstream v1.6.0; original attribution and changes are recorded in [UPSTREAM.md](https://www.github.com/vandman/pareflux-traefik-gate/blob/v0.1.17/UPSTREAM.md). PareFlux uses its own Apache-2.0 TTL cache.

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
