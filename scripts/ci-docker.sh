#!/usr/bin/env sh

###############################################################################
# CI Docker executor adapter
# FR: Sélectionne le daemon DinD, le socket local ou Docker via sudo selon les
#     capacités du runner, sans modifier les permissions du socket système.
# EN: Selects DinD, the local socket, or Docker through sudo according to the
#     runner capabilities without changing system socket permissions.
# Called by: GitLab verify, integration and public release jobs
# Calls: Docker CLI, env, timeout and optional passwordless sudo
###############################################################################

set -eu

###############################################################################
# detect_mode
# FR: Détecte une fois le mode Docker utilisable par le runner courant.
# EN: Detects once which Docker access mode works on the current runner.
# Called by: probe command and Docker command execution
# Calls: docker version and optional sudo
###############################################################################
detect_mode() {
  probe_timeout="${PAREFLUX_DOCKER_PROBE_TIMEOUT:-8}"
  if timeout "$probe_timeout" docker version >/dev/null 2>&1; then
    printf '%s\n' direct
    return
  fi
  if env -u DOCKER_HOST -u DOCKER_TLS_CERTDIR \
    timeout "$probe_timeout" docker version >/dev/null 2>&1; then
    printf '%s\n' local
    return
  fi
  if command -v sudo >/dev/null 2>&1 &&
    env -u DOCKER_HOST -u DOCKER_TLS_CERTDIR \
      timeout "$probe_timeout" sudo -n docker version >/dev/null 2>&1; then
    printf '%s\n' sudo
    return
  fi
  echo "no usable Docker daemon is available to the CI runner" >&2
  exit 1
}

if [ "${1:-}" = probe ]; then
  detect_mode
  exit 0
fi

mode="${PAREFLUX_CI_DOCKER_MODE:-$(detect_mode)}"
case "$mode" in
  direct)
    exec docker "$@"
    ;;
  local)
    exec env -u DOCKER_HOST -u DOCKER_TLS_CERTDIR docker "$@"
    ;;
  sudo)
    exec env -u DOCKER_HOST -u DOCKER_TLS_CERTDIR sudo -n docker "$@"
    ;;
  *)
    echo "unsupported CI Docker mode: $mode" >&2
    exit 1
    ;;
esac
