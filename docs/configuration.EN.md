<!--
FR: Référence des options vérifiées contre le constructeur et leur utilisation.
EN: Reference of options verified against constructor defaults and runtime usage.
Called by: README and LISEZMOI; Calls: configuration.New and runtime settings.
-->
# Configuration reference

[Installation](../README.md) · [Français](configuration.FR.md)

These are the JSON/YAML option names and default values.
Durations are seconds unless stated
otherwise; list types use YAML sequences. A `File` setting takes precedence
over its inline counterpart and refers to a file readable **inside Traefik**.
Empty values do not necessarily mean a feature is usable without credentials.
AppSec inherits LAPI scheme and TLS unless overridden; in LAPI modes it also
inherits the LAPI key. In `alone` mode, provide a separate AppSec bouncer key
if HTTP inspection is enabled: Central API credentials are not an AppSec key.

Copy only the settings you need; the installation guide sets `stream` explicitly.


## General

| Option | Type | Default | Purpose |
| --- | --- | --- | --- |
| `enabled` | `bool` | `false` | Enable protection; disabled middleware passes requests through. |
| `logLevel` | `string` | `"INFO"` | Logging level: DEBUG, INFO, WARN or ERROR. |
| `logFormat` | `string` | `"common"` | Log format: common (text) or json. |
| `logFilePath` | `string` | `""` | Log destination file; empty uses standard output. |
| `crowdsecMode` | `string` | `"live"` | Decision mode: stream, live, none, appsec or alone. |
| `updateIntervalSeconds` | `int64` | `60` | Stream refresh seconds; alone mode overrides this with 7200 seconds. |
| `metricsUpdateIntervalSeconds` | `int64` | `600` | Remediation metrics reporting interval to CrowdSec; zero disables the worker. |
| `updateMaxFailure` | `int64` | `3` | Allowed consecutive stream update failures before unhealthy; 0 means first failure, -1 disables that transition. |
| `defaultDecisionSeconds` | `int64` | `60` | Maximum live cache lifetime in seconds, bounded by the decision duration. |
| `remediationStatusCode` | `int` | `403` | HTTP status for security bans; technical errors use 408, 413 or 503 independently. |
| `httpTimeoutSeconds` | `int64` | `10` | LAPI/CAPI HTTP timeout in seconds. |
| `streamDecisionCapacity` | `int64` | `262144` | Active decision limit (maximum 1000000); no active-ban eviction, fail closed on overflow. |
| `failureGraceSeconds` | `int64` | `60` | Grace after the last successful stream synchronization or AppSec inspection, subject to failure thresholds. |

## LAPI and TLS

| Option | Type | Default | Purpose |
| --- | --- | --- | --- |
| `crowdsecLapiScheme` | `string` | `"http"` | LAPI transport: http or https. |
| `crowdsecLapiHost` | `string` | `"crowdsec:8080"` | LAPI hostname and port, reachable from Traefik. |
| `crowdsecLapiPath` | `string` | `"/"` | Base URL path before the LAPI v1 endpoints. |
| `crowdsecLapiKey` | `string` | `""` | Bouncer API key; prefer its File counterpart in production. |
| `crowdsecLapiKeyFile` | `string` | `""` | Read crowdsecLapiKey from this file inside Traefik; takes precedence over the inline value. |
| `crowdsecLapiTlsInsecureVerify` | `bool` | `false` | Disable server certificate verification; keep false in production. |
| `crowdsecLapiTlsCertificateAuthority` | `string` | `""` | PEM certificate authority used to verify the server. |
| `crowdsecLapiTlsCertificateAuthorityFile` | `string` | `""` | Read crowdsecLapiTlsCertificateAuthority from this file inside Traefik; takes precedence over the inline value. |
| `crowdsecLapiTlsCertificateBouncer` | `string` | `""` | PEM client certificate for mutual TLS authentication. |
| `crowdsecLapiTlsCertificateBouncerFile` | `string` | `""` | Read crowdsecLapiTlsCertificateBouncer from this file inside Traefik; takes precedence over the inline value. |
| `crowdsecLapiTlsCertificateBouncerKey` | `string` | `""` | PEM private key matching the client certificate. |
| `crowdsecLapiTlsCertificateBouncerKeyFile` | `string` | `""` | Read crowdsecLapiTlsCertificateBouncerKey from this file inside Traefik; takes precedence over the inline value. |
| `crowdsecStreamUnreachableBlock` | `bool` | `true` | Refuse unknown clients after the stream failure threshold and grace; unexpired known bans remain enforced. |

## AppSec

| Option | Type | Default | Purpose |
| --- | --- | --- | --- |
| `crowdsecAppsecEnabled` | `bool` | `false` | Enable HTTP inspection; required even in appsec mode. |
| `crowdsecAppsecScheme` | `string` | `""` | Empty inherits LAPI scheme and TLS settings; otherwise uses AppSec transport settings. |
| `crowdsecAppsecHost` | `string` | `"crowdsec:7422"` | AppSec hostname and port, reachable from Traefik. |
| `crowdsecAppsecPath` | `string` | `"/"` | AppSec inspection endpoint path. |
| `crowdsecAppsecKey` | `string` | `""` | AppSec API key; empty inherits the LAPI key in LAPI modes. |
| `crowdsecAppsecKeyFile` | `string` | `""` | Read crowdsecAppsecKey from this file inside Traefik; takes precedence over the inline value. |
| `crowdsecAppsecTlsInsecureVerify` | `bool` | `false` | Disable server certificate verification; keep false in production. |
| `crowdsecAppsecTlsCertificateAuthority` | `string` | `""` | PEM certificate authority used to verify the server. |
| `crowdsecAppsecTlsCertificateAuthorityFile` | `string` | `""` | Read crowdsecAppsecTlsCertificateAuthority from this file inside Traefik; takes precedence over the inline value. |
| `crowdsecAppsecTlsCertificateBouncer` | `string` | `""` | PEM client certificate for mutual TLS authentication. |
| `crowdsecAppsecTlsCertificateBouncerFile` | `string` | `""` | Read crowdsecAppsecTlsCertificateBouncer from this file inside Traefik; takes precedence over the inline value. |
| `crowdsecAppsecTlsCertificateBouncerKey` | `string` | `""` | PEM private key matching the client certificate. |
| `crowdsecAppsecTlsCertificateBouncerKeyFile` | `string` | `""` | Read crowdsecAppsecTlsCertificateBouncerKey from this file inside Traefik; takes precedence over the inline value. |
| `crowdsecAppsecFailureBlock` | `bool` | `true` | Refuse HTTP 502/503/504 after grace; false permits those errors. HTTP 500 remains blocking. |
| `crowdsecAppsecUnreachableBlock` | `bool` | `true` | Refuse connection establishment failures after grace; false permits them. Connected timeouts remain blocking. |
| `crowdsecAppsecBodyLimit` | `int64` | `1048576` | Maximum inspected body prefix in bytes; zero disables body inspection, not metadata checks. |
| `crowdsecAppsecBodyLimitAction` | `string` | `"reject"` | Oversize policy: reject with 413, or explicit partial inspection for this middleware. |
| `crowdsecAppsecBodyExceptions` | `[]object` | `[]` | Up to 32 partial-inspection upload exceptions: exact host, anchored pathRegexp and POST/PUT/PATCH methods. |
| `crowdsecAppsecBodyExcludedContentTypes` | `[]string` | `[]` | Compatibility only; no longer bypasses inspection. |
| `crowdsecAppsecReadTimeoutSeconds` | `int64` | `60` | Finite body deadline after inspection; align with Traefik, 0 leaves deadlines untouched. |
| `crowdsecAppsecMaxConcurrentPerClient` | `int64` | `4` | Buffered bodies per IPv4 or IPv6 /64; IPv6 /48 allows four times this quota. Maximum 64; 0 uses 4; bodyless requests are not counted. |
| `crowdsecAppsecTimeoutSeconds` | `int64` | `3` | Total budget for body reading and AppSec inspection; default 3 seconds, 0 also uses 3. |

## Client identity and logs

| Option | Type | Default | Purpose |
| --- | --- | --- | --- |
| `traceHeadersCustomName` | `string` | `""` | Request header exposed as TraceID to a custom ban template. |
| `remediationHeadersCustomName` | `string` | `""` | Sanitized internal request and rejection-response marker; retain it in access logs to support loop filtering. |
| `forwardedHeadersCustomName` | `string` | `"X-Forwarded-For"` | Header used for client identity only when the immediate peer is trusted. |
| `forwardedHeadersTrustedIps` | `[]string` | `[]` | Trusted proxy IPs/CIDRs; these do not exempt visitors from protection. |
| `clientTrustedIps` | `[]string` | `[]` | Resolved client IPs/CIDRs bypassing all Gate checks, including AppSec. |

## Redis

| Option | Type | Default | Purpose |
| --- | --- | --- | --- |
| `redisCacheEnabled` | `bool` | `false` | Use Redis for live lookups and captcha; stream/alone decisions remain in the separate in-memory store. |
| `redisCacheHost` | `string` | `"redis:6379"` | Redis hostname and port. |
| `redisCachePassword` | `string` | `""` | Redis password; prefer the File counterpart. |
| `redisCachePasswordFile` | `string` | `""` | Read redisCachePassword from this file inside Traefik; takes precedence over the inline value. |
| `redisCacheDatabase` | `string` | `""` | Redis database number as a string; empty uses the default database. |
| `redisCacheUnreachableBlock` | `bool` | `true` | Reject a request needing a Redis lookup when Redis cannot be reached; stream IP lookups do not use Redis. |

## Captcha and ban templates

| Option | Type | Default | Purpose |
| --- | --- | --- | --- |
| `banHtmlFilePath` | `string` | `""` | Optional Go HTML template for ban responses; empty returns status only. |
| `captchaHtmlFilePath` | `string` | `"/captcha.html"` | Go HTML template used for captcha responses; mount it inside Traefik. |
| `captchaProvider` | `string` | `""` | Captcha provider: hcaptcha, recaptcha, turnstile or custom; empty disables challenges. |
| `captchaCustomJsUrl` | `string` | `""` | JavaScript URL for the custom captcha provider. |
| `captchaCustomValidateUrl` | `string` | `""` | Server-side verification URL for the custom provider. |
| `captchaCustomKey` | `string` | `""` | HTML element class expected by the custom provider. |
| `captchaCustomResponse` | `string` | `""` | Form response field expected by the custom provider. |
| `captchaSiteKey` | `string` | `""` | Public captcha site key. |
| `captchaSiteKeyFile` | `string` | `""` | Read captchaSiteKey from this file inside Traefik; takes precedence over the inline value. |
| `captchaSecretKey` | `string` | `""` | Captcha verification secret. |
| `captchaSecretKeyFile` | `string` | `""` | Read captchaSecretKey from this file inside Traefik; takes precedence over the inline value. |
| `captchaGracePeriodSeconds` | `int64` | `1800` | Seconds before another captcha is required after successful validation. |

## Central API

| Option | Type | Default | Purpose |
| --- | --- | --- | --- |
| `crowdsecCapiMachineId` | `string` | `""` | Registered machine identifier for alone mode. |
| `crowdsecCapiMachineIdFile` | `string` | `""` | Read crowdsecCapiMachineId from this file inside Traefik; takes precedence over the inline value. |
| `crowdsecCapiPassword` | `string` | `""` | Machine password for alone mode. |
| `crowdsecCapiPasswordFile` | `string` | `""` | Read crowdsecCapiPassword from this file inside Traefik; takes precedence over the inline value. |
| `crowdsecCapiScenarios` | `[]string` | `[]` | Scenario list sent when registering/authenticating with CAPI. |

## Dashboard integration

| Option | Type | Default | Purpose |
| --- | --- | --- | --- |
| `dashboardHealthURL` | `string` | `""` | Private Control readiness URL; never a browser credential or public SSO login page. |
| `dashboardIntegration` | `bool` | `false` | Enable private URL discovery on a separate navigation middleware with enabled=false. |
| `dashboardLinkURL` | `string` | `""` | Public Control HTTPS URL or safe absolute path; deployment-derived initial link. |
| `dashboardHost` | `string` | `""` | Exact native dashboard hostname; no scheme or path. |
| `dashboardDiscoveryKey` | `string` | `""` | Shared HMAC key, at least 32 bytes; prefer a secret file. |
| `dashboardDiscoveryKeyFile` | `string` | `""` | Read the shared discovery key from a file inside Traefik. |
| `dashboardRegistrationAllowed` | `bool` | `false` | Enable signed registration only on the private navigation instance. |

[Runtime limits and policies](security-runtime.EN.md)
