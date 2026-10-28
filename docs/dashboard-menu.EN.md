<!-- FR: Décrit la navigation privée signée et sa configuration. EN: Describes signed private navigation and its setup. Called by: operators; Calls: Gate and Control integration. -->
# Native dashboard navigation

[Installation](../README.md) · [Français](dashboard-menu.FR.md)

This optional integration adds a PareFlux link to Traefik’s dashboard and a
Traefik return link in Control. It does not move either application: the
dashboard service remains `api@internal`. Gate protects without the console.
Authentication may use SSO or another suitable mechanism; private exchanges
never transmit user sessions.

## Configure both sides

Mount the same random key of at least 32 bytes read-only in Traefik and Control.
Generate it outside Git-tracked files, for example with `openssl rand -hex 32`.
Do not reuse the LAPI key for this exchange.

In Control, adapt these variables to its actual routes:

```yaml
PAREFLUX_HOST: security.example.org
PAREFLUX_BASE_PATH: /console
PAREFLUX_INGRESS_INTEGRATION_URL: http://traefik-private:9000
PAREFLUX_INGRESS_DISCOVERY_KEY_FILE: /run/secrets/pareflux-discovery.key
```

File paths are internal to each container. `PAREFLUX_HOST` has no scheme or
path. The integration URL is the private Traefik origin; do not append
`/.well-known/pareflux`, which Control adds itself.

Declare a private entrypoint in Traefik’s **static** configuration:

```yaml
entryPoints:
  pareflux-private:
    address: ":9000"
```

Do not publish this port on the Internet. Restrict it to Control through a
private network, firewall or NetworkPolicy. Signing does not replace this restriction.

In **dynamic** configuration, adapt this example to the existing dashboard.
`dashboard-auth` represents its existing authentication middleware: define it
or retain the equivalent existing chain. Enable the dashboard API in static
configuration. YAML anchors copy the same engine options into both navigation
instances.

```yaml
http:
  middlewares:
    pareflux-gate:
      plugin:
        parefluxgate: &protection
          enabled: true
          crowdsecMode: stream
          crowdsecLapiHost: crowdsec:8080
          crowdsecLapiKeyFile: /run/secrets/crowdsec-bouncer.key
          crowdsecAppsecEnabled: false
    pareflux-navigation:
      plugin:
        parefluxgate: &navigation
          <<: *protection
          enabled: false
          dashboardIntegration: true
          dashboardHost: traefik.example.org
          dashboardHealthURL: http://pareflux-control:8080/console/readyz
          dashboardDiscoveryKeyFile: /run/secrets/pareflux-discovery.key
          dashboardRegistrationAllowed: false
    pareflux-registration:
      plugin:
        parefluxgate:
          <<: *navigation
          dashboardRegistrationAllowed: true
  routers:
    dashboard:
      rule: Host(`traefik.example.org`) && (PathPrefix(`/dashboard`) || PathPrefix(`/api`))
      entryPoints: [websecure]
      tls: {}
      service: api@internal
      middlewares: [dashboard-auth, pareflux-gate, pareflux-navigation]
    pareflux-private:
      rule: Path(`/.well-known/pareflux`)
      entryPoints: [pareflux-private]
      service: api@internal
      middlewares: [pareflux-registration]
```

Navigation instances use `enabled: false`: they do not call CrowdSec or replace
the protection middleware. Keep exactly the engine options actually used for
protection, including after configuration changes or key rotation, so protection
evidence matches. Only the private instance accepts signed announcements. The
public instance keeps `dashboardRegistrationAllowed: false`. Without a valid
shared key, `dashboardIntegration` cannot be constructed.

The domains above are examples; adapt all addresses to your installation.

## Display and status

Successful uncompressed HTML at `/dashboard/` receives a link under “PLUGINS”
and a “PareFlux / CrowdSec” tile under “Features”. Assets, errors, redirects
and APIs are not rewritten. The HTML buffer is limited to 2 MiB; larger or
streaming responses pass through. A floating link is the fallback if the native
structure is not recognized. `/dashboard` redirects to `/dashboard/`, retaining
the query, after the route’s checks.

“Connected” means Control is ready. “Protected” requires actual Gate processing,
recent CrowdSec contact and successful AppSec inspection when enabled. These
states remain separate, with color, text and tooltips. Status refreshes every
15 seconds on a dashboard route protected by the same chain. Browser-supplied
headers cannot confirm protection.

The private probe has a 750 ms timeout and a 15-second result cache. Only HTTP
200 with JSON `status: ready` counts, not a login page. A slow probe does not
hold the shared lock during its network call. While the announcement is valid,
an outage keeps a warning link with a tooltip in the 24 official EU languages
or Korean. **After 90 seconds without renewal, the link is removed**, even if
an old health probe succeeded. The dashboard remains independent of Control.

## Mutual discovery protocol

Every 30 seconds Control sends a private POST to `/.well-known/pareflux` with
`control_url`, `health_path` and a random `nonce`. Requests and responses carry
`X-Pareflux-Time` and `X-Pareflux-Signature`: an HMAC signature binds the body,
timestamp and exchange direction. Invalid signatures, replayed announcements
and expired timestamps are rejected.

The response includes `protocol: pareflux.ingress.v1`, `name`, `dashboard_url`,
`control_url`, `nonce`, `expires_in: 90`, `control_reachable`, `gate_version` and
`range_decisions`. The latter confirms only the matching healthy, fresh engine.
Control displays the ingress return link only after a valid exchange and hides
it on expiration.

The console can announce a new public URL. Its health path remains resolved
against the configured private origin: an announcement cannot choose another
probe server. Without a valid announcement, discovery displays no link.
For a simple static link without discovery, set `dashboardLinkURL` and
`dashboardHost` with `enabled: false`, leaving `dashboardIntegration: false`;
that mode does not provide the same status exchange. Other ingress plugins
may implement the protocol; they are not provided here.
