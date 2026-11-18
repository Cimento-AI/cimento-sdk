#!/usr/bin/env bash
# Prints the version of an npm package now tagged latest, or nothing when the
# package has never been published.
#
#   npm-latest.sh @cimento/webhooks
#
# Exits 1 when the registry does not answer either way. "Could not find out"
# must never be taken for "not published": a release would then be tagged
# latest over a newer one.
set -euo pipefail

name="${1-}"
registry="${NPM_REGISTRY:-https://registry.npmjs.org}"

if [ -z "$name" ]; then
  echo "npm-latest: no package named" >&2
  exit 1
fi

body="$(mktemp)"
trap 'rm -f "$body"' EXIT

# The package's own document, in its short form. The registry answers 404 for
# a package that does not exist here, and not on its dist-tags endpoint, which
# answers 401 for one in a scope. A scope's slash is written %2f.
url="$registry/${name//\//%2f}"
if ! status="$(curl --silent --show-error --retry 3 --max-time 30 --header 'Accept: application/vnd.npm.install-v1+json' --output "$body" --write-out '%{http_code}' "$url")"; then
  echo "npm-latest: the registry could not be reached" >&2
  exit 1
fi

case "$status" in
  200)
    latest="$(jq -er '."dist-tags".latest | strings' "$body")" || {
      echo "npm-latest: the registry's answer for $name has no latest" >&2
      exit 1
    }
    printf '%s\n' "$latest"
    ;;
  404) ;;
  *)
    echo "npm-latest: the registry answered $status for $name" >&2
    exit 1
    ;;
esac
