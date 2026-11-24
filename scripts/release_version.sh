#!/usr/bin/env bash

set -euo pipefail

# The application build version uses YY.MM.N[-rc.K]. Cat_two Docker Hub numbering
# is handled by scripts/cat_two_docker_release.py.
parse_release_tag() {
  local tag="$1"
  if [[ ! "$tag" =~ ^([1-9][0-9]\.(0[1-9]|1[0-2]))\.(0|[1-9][0-9]*)(-rc\.([1-9][0-9]*))?$ ]]; then
    return 1
  fi
  RELEASE_VERSION="$tag"
  RELEASE_SERIES="${BASH_REMATCH[1]}"
  RELEASE_IS_PRERELEASE=false
  if [[ -n "${BASH_REMATCH[5]}" ]]; then
    RELEASE_IS_PRERELEASE=true
  fi
}

# development_version prints YY.MM.D-dev.<sha7> for HEAD from its committer date
# in UTC, matching internal/version. format-local honors TZ; plain format uses
# the commit's original zone.
development_version() {
  local repo="$1" head month day sha version
  head="$(TZ=UTC git -C "$repo" show -s --format='%cd %H' --date='format-local:%y.%m %d' HEAD)"
  read -r month day sha <<<"$head"
  version="$month.${day#0}"
  if ! parse_release_tag "$version"; then
    echo "Cannot derive a calendar version from HEAD" >&2
    return 1
  fi
  printf '%s-dev.%s\n' "$version" "${sha:0:7}"
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  case "${1:-}" in
    development-version)
      development_version "${2:-.}"
      ;;
    version)
      tag="${2:?release tag required}"
      if ! parse_release_tag "$tag"; then
        echo "Unsupported release tag format: $tag" >&2
        exit 1
      fi
      printf 'tag=%s\nversion=%s\nseries=%s\nis_prerelease=%s\n' \
        "$tag" "$RELEASE_VERSION" "$RELEASE_SERIES" "$RELEASE_IS_PRERELEASE"
      ;;
    *) echo "Unknown release command: ${1:-}" >&2; exit 1 ;;
  esac
fi
