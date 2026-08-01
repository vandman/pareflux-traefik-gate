#!/usr/bin/env bash
###############################################################################
# PareFlux dependency update reporter
# FR: Liste les versions epinglees et signale les mises a jour Go et amont.
# EN: Lists pinned versions and reports available Go and upstream updates.
# Called by: developers, CI scheduled jobs
# Calls: go, curl, jq, git
###############################################################################
set -euo pipefail
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${repo_root}"
printf '%s\n' "Pinned dependencies:"
rg -n --glob 'go.mod' --glob 'Dockerfile*' --glob '*.yml' --glob '*.yaml' '(^go [0-9]|^[[:space:]]*[A-Za-z0-9._/-]+ v[0-9]|^FROM [^ ]+:[^ ]+|image:[[:space:]]*[^ ]+:[^ ]+|ref:[[:space:]]*[^ ]+)' . | rg -v '(^|/)(vendor|.git)/' || true
if [[ -f go.mod ]]; then
  printf '\n%s\n' "Available Go module updates:"
  go list -mod=mod -m -u -f '{{if .Update}}{{.Path}} {{.Version}} -> {{.Update.Version}}{{end}}' all | sed '/^[[:space:]]*$/d' || true
fi
if [[ -f versions.conf ]]; then
  printf '\n%s\n' "Tracked upstream releases:"
  while IFS='|' read -r kind name current source; do
    [[ -z "${kind}" || "${kind}" == \#* ]] && continue
    case "${kind}" in
      github) latest="$(curl -fsSL "https://api.github.com/repos/${source}/releases/latest" | jq -r '.tag_name')" ;;
      git) latest="$(git ls-remote --tags --refs "${source}" | awk -F/ '{print $NF}' | sort -V | tail -1)" ;;
      *) printf '%s\n' "Unsupported source type: ${kind}" >&2; exit 2 ;;
    esac
    printf '%s %s -> %s\n' "${name}" "${current}" "${latest:-unavailable}"
  done < versions.conf
fi
