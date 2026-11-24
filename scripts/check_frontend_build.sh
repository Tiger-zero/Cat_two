#!/usr/bin/env sh

set -eu

dist="${1:?pass the path of the embedded frontend assets}"
index="$dist/index.html"
if [ -f "$index" ] && grep -Fq 'id="root"' "$index"; then
  entry="$(sed -nE 's#.*<script [^>]*src="/(assets/[a-zA-Z0-9._-]+\.js)"[^>]*>.*#\1#p' "$index")"
  if [ -n "$entry" ] && [ -s "$dist/$entry" ]; then
    exit 0
  fi
fi

echo 'The production frontend is missing or incomplete. Run (cd web && pnpm release) before packaging.' >&2
exit 1
