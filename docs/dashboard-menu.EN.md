# Native dashboard navigation

Use a separate instance of the PareFlux plugin after the existing security and
SSO middlewares on the Traefik dashboard router. Keep the existing security
instance enabled. The navigation instance uses `enabled: false` and
`dashboardIntegration: true`; it neither calls CrowdSec nor replaces protection.
The dashboard service remains `api@internal`. PareFlux Control serves no dashboard HTML.

Configure `dashboardHost` with the dashboard's exact host, `dashboardLinkURL`
with the console's public URL and `dashboardHealthURL` with its private readiness
endpoint. These are deployment inputs; the application contains no site URLs.
The deployment helper derives them from existing router rules, Control environment
and Service data. For example, a different installation may publish the console
at `https://security.example/console/`.

Only successful uncompressed dashboard entry HTML receives the link. Assets,
APIs, errors, bans and redirects stay unchanged. Buffering is bounded to 2 MiB;
large or streaming responses pass through. `/dashboard` redirects locally to
`/dashboard/`, preserving the query after SSO authorization.

The private readiness check has a 750 ms timeout and a 15-second cache. Only
HTTP 200 with JSON `status: ready` counts as ready; an SSO page does not. An
unavailable service keeps a clickable last-known link marked with a warning
and a tooltip in the 24 official EU languages or Korean according to the
browser's language. The native dashboard remains independent of Control.

The native “PLUGINS” section contains the PareFlux link. The “PareFlux /
CrowdSec” tile in “Features” separates “Connected” (console ready) from
“Protected” (actual Gate processing, recent CrowdSec contact and successful
AppSec inspection when enabled). Confirmed states are green, other states red,
with text and tooltips. Status refreshes every 15 seconds through a dashboard
route protected by the existing middleware chain. Browser-supplied headers
cannot confirm protection. A floating link remains available when the native
page structure cannot be recognized.

## Mutual discovery protocol

Expose `POST /.well-known/pareflux` only on an internal entrypoint restricted
to Control by network policy. Never publish it through the public dashboard route.
Control sends JSON `control_url`, `health_path` and a random `nonce` every 30 seconds.
The plugin announces protocol `pareflux.ingress.v1`, product `name`, `dashboard_url`,
accepted `control_url`, echoed `nonce`, `expires_in: 90` and `control_reachable`.
No user cookies or SSO credentials participate. Control displays the ingress
return link only after a valid exchange and hides it when absent or expired.

The console may announce a new public URL without rebuilding the plugin. Its
readiness path may change too; it is resolved only against the configured private
health origin, preventing a request from choosing another health server.
The last-announced console URL remains available with a warning during outages.
An instance without an integration endpoint is standalone and has no return link.
Other ingress plugins can implement this same protocol; they are not provided here.
