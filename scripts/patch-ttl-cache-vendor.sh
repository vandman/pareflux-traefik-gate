#!/bin/sh
###############################################################################
# Bounded TTL cache expiry patch
# FR: Réapplique une modification locale vérifiable après go mod vendor.
# EN: Reapplies a verifiable local modification after go mod vendor.
# Called by: GitLab CI and maintainers; Calls: sha256sum, git apply.
###############################################################################
set -eu
cd "$(dirname "$0")/.."
file=vendor/gitlab.nikolov.fr/application/pareflux-ttl-cache/cache.go
metadata=patches/ttl-cache-expiry.json
original=$(sed -n 's/.*"upstream_sha256": "\([a-f0-9]*\)".*/\1/p' "$metadata")
patched=$(sed -n 's/.*"patched_sha256": "\([a-f0-9]*\)".*/\1/p' "$metadata")
current=$(sha256sum "$file" | cut -d ' ' -f 1)
[ "$current" != "$patched" ] || exit 0
[ "$current" = "$original" ] || { echo 'Unexpected TTL cache source; review the patch provenance' >&2; exit 1; }
git apply --unidiff-zero --check patches/ttl-cache-expiry.patch
git apply --unidiff-zero patches/ttl-cache-expiry.patch
[ "$(sha256sum "$file" | cut -d ' ' -f 1)" = "$patched" ]
