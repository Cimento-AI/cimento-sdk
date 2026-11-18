#!/usr/bin/env bash
# Checks that a version is one Go accepts for a module whose path has no major
# version in it, and prints it as the module proxy spells it in a URL.
#
#   go-version.sh v0.1.0-RC.1    prints v0.1.0-!r!c.1
#
# Exits 1, printing nothing, when the version is not acceptable.
set -euo pipefail

version="${1-}"

# One part of a pre-release: a number with no leading zero, or anything with a
# letter or a hyphen in it.
part='(0|[1-9][0-9]*|[0-9]*[A-Za-z-][0-9A-Za-z-]*)'
if ! printf '%s\n' "$version" | grep -Eqx "v[01]\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-$part(\.$part)*)?"; then
  exit 1
fi

# The proxy writes a capital letter as "!" and the small letter.
printf '%s\n' "$version" | sed 's/[A-Z]/!&/g' | tr '[:upper:]' '[:lower:]'
