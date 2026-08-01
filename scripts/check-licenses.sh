#!/usr/bin/env sh
###############################################################################
# Dependency license gate
# FR: Refuse les modules interdits et vérifie que chaque dépendance vendored
#     appartient à la liste permissive approuvée.
# EN: Rejects forbidden modules and verifies that every vendored dependency
#     belongs to the approved permissive allowlist.
# Called by: GitLab CI and maintainers
# Calls: awk, grep, sort, test
###############################################################################
set -eu

if grep -R -E -i \
  'github.com/leprosus/golang-ttl-map|GNU (AFFERO )?GENERAL PUBLIC LICENSE' \
  go.mod go.sum vendor 2>/dev/null; then
  echo "forbidden dependency or license detected" >&2
  exit 1
fi

expected_modules='gitlab.nikolov.fr/application/pareflux-ttl-cache'

actual_modules=$(awk '/^# / && $3 ~ /^v/ { print $2 }' vendor/modules.txt | sort)
if [ "$actual_modules" != "$(printf '%s\n' "$expected_modules" | sort)" ]; then
  echo "vendored module allowlist mismatch" >&2
  exit 1
fi
