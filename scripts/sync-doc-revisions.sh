#!/usr/bin/env bash
# Sync :revnumber: and :revdate: in docs/*.adoc from the latest release tag.
# Run at release time after tagging (or via: make sync-doc-revisions).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"

revnumber="$(git -C "${ROOT}" describe --tags --abbrev=0 --match 'v*.*.*' 2>/dev/null || true)"
if [[ -z "${revnumber}" ]]; then
  echo "No release tag (vX.Y.Z) found" >&2
  exit 1
fi
revdate="$(git -C "${ROOT}" log -1 --format=%cs "${revnumber}")"

shopt -s nullglob
files=("${ROOT}"/docs/*.adoc)
if [[ ${#files[@]} -eq 0 ]]; then
  echo "No docs/*.adoc files found" >&2
  exit 1
fi

for f in "${files[@]}"; do
  tmp="$(mktemp)"
  # The revision attributes go right after the author line, the first line
  # after the title that is not a preprocessor directive such as the
  # ifndef::book[] guard around a chapter's header.
  awk -v rev="${revnumber}" -v date="${revdate}" '
    /^:revnumber:/ { next }
    /^:revdate:/ { next }
    {
      print
      if (!inserted && seen_title && !/^(ifn?def|endif)::/) {
        print ":revnumber: " rev
        print ":revdate: " date
        inserted = 1
      }
      if (/^= /) seen_title = 1
    }
    END {
      if (!inserted) exit 1
    }
  ' "${f}" > "${tmp}" || {
    rm -f "${tmp}"
    echo "Could not insert revision attributes in ${f} (no document title and author line found)" >&2
    exit 1
  }
  mv "${tmp}" "${f}"
  echo "Updated $(basename "${f}") → ${revnumber}, ${revdate}"
done
