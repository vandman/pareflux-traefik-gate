<!--
###############################################################################
# PareFlux Gate security policy
# FR: Definit le canal prive et les informations attendues pour signaler une
#     vulnerabilite sans exposer les utilisateurs du plugin.
# EN: Defines the private channel and expected information for reporting a
#     vulnerability without exposing plugin users.
# Called by: Researchers, users, and GitHub security interface
# Calls: GitHub private vulnerability reporting and PareFlux maintainers
###############################################################################
-->

# Security policy

## Supported versions

Security fixes target the latest published PareFlux Gate release. Confirm the
problem against that release before reporting it whenever this can be done
safely.

## Reporting a vulnerability

Do not open a public issue for a vulnerability, exploit, authentication or
authorization bypass, secret exposure, unsafe client identity resolution, or
another security-sensitive weakness.

Use [GitHub private vulnerability reporting](https://github.com/vandman/pareflux-traefik-gate/security/advisories/new).
Include:

- the affected version and Traefik version;
- the security impact and realistic threat model;
- minimal reproduction steps or a proof of concept;
- sanitized configuration and relevant logs;
- known mitigations or workarounds;
- your preferred disclosure attribution, if any.

Never send live credentials, tokens, cookies, private keys, personal data, or
unnecessary identifying information. Maintainers will coordinate validation,
remediation, and responsible disclosure through the private advisory.

For non-sensitive hardening proposals, use a public feature request instead.

The French version is available in [SECURITY.FR.md](SECURITY.FR.md).
