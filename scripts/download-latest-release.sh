#!/usr/bin/env bash
# Download every asset from the latest Forgejo release of this repo.
#
# Required:
#   FORGEJO_TOKEN   Forgejo access token with read:repository
#
# Optional:
#   FORGEJO_URL     Forgejo instance (default: https://git.smartowl.nl)
#   FORGEJO_REPO    owner/repo        (default: Development/tuios)
#   DEST            output directory  (default: current directory)

set -euo pipefail

FORGEJO_URL="${FORGEJO_URL:-https://git.smartowl.nl}"
FORGEJO_REPO="${FORGEJO_REPO:-Development/tuios}"
DEST="${DEST:-.}"

if [ -z "${FORGEJO_TOKEN:-}" ]; then
	echo "FORGEJO_TOKEN is not set" >&2
	echo "Create a token with read:repository and export it, e.g.:" >&2
	echo "  export FORGEJO_TOKEN=..." >&2
	exit 1
fi

if ! command -v curl >/dev/null 2>&1; then
	echo "curl is required" >&2
	exit 1
fi

api="${FORGEJO_URL%/}/api/v1/repos/${FORGEJO_REPO}"
auth_header="Authorization: token ${FORGEJO_TOKEN}"

json=$(curl -sf -H "${auth_header}" "${api}/releases/latest") || {
	echo "Failed to fetch latest release from ${api}/releases/latest" >&2
	exit 1
}

tag=$(printf '%s' "$json" | grep -o '"tag_name":"[^"]*"' | head -1 | sed 's/"tag_name":"//;s/"$//')
if [ -z "$tag" ]; then
	echo "Could not parse tag_name from API response" >&2
	exit 1
fi

mkdir -p "$DEST"

downloaded=0
while IFS= read -r url; do
	[ -n "$url" ] || continue
	name=$(basename "$url")
	echo "Downloading ${name} (${tag})..."
	curl -fL -H "${auth_header}" -o "${DEST}/${name}" "$url"
	downloaded=$((downloaded + 1))
done < <(printf '%s' "$json" | grep -o '"browser_download_url":"[^"]*"' | sed 's/"browser_download_url":"//;s/"$//')

if [ "$downloaded" -eq 0 ]; then
	echo "Latest release ${tag} has no assets" >&2
	exit 1
fi

echo "Downloaded ${downloaded} asset(s) for ${tag} into ${DEST}"
