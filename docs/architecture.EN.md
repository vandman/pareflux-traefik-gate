<!--
FR: Décrit les composants et décisions du middleware sans alourdir le guide d’installation.
EN: Describes middleware components and decisions separately from the installation guide.
Called by: README and LISEZMOI; Calls: Mermaid diagrams.
-->
# PareFlux Gate architecture

This document describes request handling, network connections, caching and failure policies. GitHub renders the Mermaid diagrams.

[Back to installation](../README.md) · [Français](architecture.FR.md)

## Architecture

```mermaid
flowchart TB
    client["HTTP client"]

    subgraph process["Traefik process"]
        entrypoint["Entrypoint and router"]

        subgraph gate["PareFlux Gate middleware"]
            identity["Source identity resolver"]
            policy["Decision and failure policy"]
            cache["PareFlux TTL Cache"]
            appsecclient["AppSec client"]
            marker["Remediation marker"]
        end

        service["Traefik service handler"]
    end

    lapi["CrowdSec LAPI<br/>decision stream"]
    appsec["CrowdSec AppSec<br/>HTTP inspection"]
    backend["Protected backend"]
    accesslog["Traefik JSON access log"]

    client --> entrypoint --> identity --> policy
    policy <--> cache
    policy <--> lapi
    policy --> appsecclient <--> appsec
    policy --> marker
    policy --> service --> backend
    entrypoint --> accesslog
```

## Network flow

```mermaid
flowchart LR
    internet["Client network"]

    subgraph traefikcontainer["Traefik container"]
        tls["Traefik entrypoints<br/>:80, :443, optional :8448"]
        gate["Gate runs in-process<br/>no listening port"]
    end

    subgraph front["Internal front network"]
        lapi["CrowdSec LAPI :8080"]
        appsec["CrowdSec AppSec :7422"]
        backend["Application service port"]
    end

    secret["Read-only LAPI key file"]
    log["Persisted access.log"]

    internet --> tls --> gate
    gate -->|"decision stream"| lapi
    gate -->|"inspection request"| appsec
    gate -->|"allowed request through Traefik"| backend
    secret --> gate
    tls --> log
```

The client never connects to Gate, LAPI, or AppSec directly. The only public
listener is a Traefik entrypoint. The trusted forwarded-header ranges determine
when Gate may derive a client identity from a proxy header instead of the
network peer.

## Application flow

```mermaid
sequenceDiagram
    participant C as Client
    participant T as Traefik router
    participant G as PareFlux Gate
    participant K as TTL cache
    participant L as CrowdSec LAPI
    participant W as CrowdSec AppSec
    participant A as Application

    L-->>G: Periodic decision stream update
    G->>K: Insert or delete expiring decisions
    C->>T: HTTP request
    T->>G: Middleware invocation
    G->>G: Resolve trusted client identity
    G->>K: Look up decision
    alt Cached ban or captcha
        K-->>G: Remediation
        G-->>C: Configured status and remediation marker
    else No cached remediation
        G->>W: Method, URI, host, headers, IP, eligible body
        alt AppSec rejects
            W-->>G: Blocking status
            G-->>C: Configured status and remediation marker
        else AppSec allows or configured fail-open applies
            W-->>G: Allow
            G->>A: Invoke next Traefik handler
            A-->>C: Application response
        end
    end
```

## Ban decision flow

```mermaid
flowchart TD
    request["Traefik invokes Gate"]
    enabled{"Gate enabled?"}
    identity["Resolve network peer or trusted forwarded identity"]
    identityok{"Identity valid?"}
    trusted{"Client IP explicitly trusted?"}
    mode{"Decision mode uses CrowdSec?"}
    lookup["Look up IP in TTL or Redis cache"]
    cached{"Cached value?"}
    remediation["Return ban or captcha remediation<br/>and internal marker"]
    stream{"Stream mode healthy?"}
    closed{"Explicit stream fail-closed enabled?"}
    technical["Return technical rejection"]
    appsec{"AppSec enabled?"}
    inspect["Send request metadata and eligible body to AppSec"]
    wafresult{"AppSec result?"}
    wafpolicy{"Failure policy blocks this error?"}
    next["Invoke the next Traefik handler"]

    request --> enabled
    enabled -->|No| next
    enabled -->|Yes| identity --> identityok
    identityok -->|No| technical
    identityok -->|Yes| trusted
    trusted -->|Yes| next
    trusted -->|No| mode
    mode -->|No or AppSec-only| appsec
    mode -->|Yes| lookup --> cached
    cached -->|Ban or captcha| remediation
    cached -->|Explicit allow| appsec
    cached -->|Miss| stream
    stream -->|Healthy or non-stream mode| appsec
    stream -->|Unhealthy| closed
    closed -->|Yes| technical
    closed -->|No, explicitly configured| appsec
    appsec -->|No| next
    appsec -->|Yes| inspect --> wafresult
    wafresult -->|Allow| next
    wafresult -->|Block| remediation
    wafresult -->|Unavailable or internal error| wafpolicy
    wafpolicy -->|Block| technical
    wafpolicy -->|Fail open| next
```

Cached bans remain effective during a LAPI outage. On a cache miss the stream
policy fails closed by default; `crowdsecStreamUnreachableBlock: false` is the
explicit fail-open switch. AppSec availability and internal-error policies are
configured independently.
