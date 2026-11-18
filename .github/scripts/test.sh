#!/usr/bin/env bash
# Tests the scripts the release workflows decide with.
set -uo pipefail
cd "$(dirname "$0")"

failed=0
expect() {
  local wanted="$1"
  shift
  local got
  got="$("$@" 2>/dev/null)" || got="refused"
  if [ "$got" != "$wanted" ]; then
    echo "FAIL: $* gave '$got', not '$wanted'"
    failed=1
  fi
}

expect v0.1.0 ./go-version.sh v0.1.0
expect v1.12.3 ./go-version.sh v1.12.3
expect v0.1.0-rc.1 ./go-version.sh v0.1.0-rc.1
expect 'v0.1.0-!r!c.1' ./go-version.sh v0.1.0-RC.1
expect 'v0.1.0-!beta.2' ./go-version.sh v0.1.0-Beta.2
expect v0.1.0-0 ./go-version.sh v0.1.0-0
expect v0.1.0-alpha.1 ./go-version.sh v0.1.0-alpha.1
expect v0.1.0-0a ./go-version.sh v0.1.0-0a
expect refused ./go-version.sh v0.1.0-01
expect refused ./go-version.sh v0.1.0-alpha.01
expect refused ./go-version.sh v0.1.0-
expect refused ./go-version.sh v0.1.0-a..b
expect refused ./go-version.sh v2.0.0
expect refused ./go-version.sh v1.x
expect refused ./go-version.sh v0.01.0
expect refused ./go-version.sh 0.1.0
expect refused ./go-version.sh v0.1.0+build
expect refused ./go-version.sh ""

expect latest ./npm-dist-tag.sh 0.1.0
expect latest ./npm-dist-tag.sh 0.1.0 0.0.0
expect latest ./npm-dist-tag.sh 0.10.0 0.9.3
expect latest ./npm-dist-tag.sh 0.2.0 0.2.0
expect next ./npm-dist-tag.sh 0.2.0-rc.1 0.1.0
expect next ./npm-dist-tag.sh 0.2.0-rc.1
expect release-0.1 ./npm-dist-tag.sh 0.1.1 0.2.0
expect release-0.9 ./npm-dist-tag.sh 0.9.4 0.10.0
expect release-1.2 ./npm-dist-tag.sh 1.2.9 2.0.0
expect refused ./npm-dist-tag.sh v0.1.0
expect refused ./npm-dist-tag.sh ""

if [ "$failed" = 0 ]; then
  echo "release scripts: ok"
fi
exit "$failed"
