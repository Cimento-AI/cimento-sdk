#!/usr/bin/env bash
# Prints the npm distribution tag a version is published under.
#
#   npm-dist-tag.sh <version> [<the version now tagged latest>]
#
#   a pre-release (0.2.0-rc.1)                    next
#   newer than latest, or nothing is published    latest
#   older than latest (0.1.1 after 0.2.0)         release-0.1
#
# npm refuses to publish a pre-release, or a version older than latest,
# without being told the tag. And neither should become what
# `npm install @cimento/webhooks` gives.
set -euo pipefail

version="${1-}"
latest="${2-}"

if ! printf '%s\n' "$version" | grep -Eqx '(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?'; then
  echo "npm-dist-tag: $version is not a version" >&2
  exit 1
fi

case "$version" in
  *-*)
    echo next
    exit 0
    ;;
esac

if [ -z "$latest" ] || [ "$version" = "$latest" ]; then
  echo latest
  exit 0
fi

# latest is never a pre-release, so sorting the two as versions is enough.
newest="$(printf '%s\n%s\n' "$version" "$latest" | sort -V | tail -n 1)"
if [ "$newest" = "$version" ]; then
  echo latest
else
  echo "release-${version%.*}"
fi
