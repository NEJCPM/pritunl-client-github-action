#!/usr/bin/env bash
# Adds or updates a Pritunl Client version in pkg/checksum/manifest.json.
#
# Usage:
#   scripts/update-checksum-manifest.sh <version>      # e.g. 1.3.5000.0
#
# Downloads the official artifacts, computes SHA-256 and rewrites the
# manifest. Run `go test ./pkg/checksum/` afterwards and commit the diff.
set -euo pipefail

if [ $# -ne 1 ]; then
  echo "usage: $0 <version>" >&2
  exit 1
fi

version="$1"
if ! [[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "error: version must be numeric dot-separated (e.g. 1.3.5000.0), got: $version" >&2
  exit 1
fi

repo_root="$(cd "$(dirname "$0")/.." && pwd)"
manifest="$repo_root/pkg/checksum/manifest.json"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

base_url="https://github.com/pritunl/pritunl-client-electron/releases/download/${version}"
declare -A artifacts=(
  ["deb-noble-amd64"]="${base_url}/pritunl-client_${version}-0ubuntu1.noble_amd64.deb"
  ["deb-jammy-amd64"]="${base_url}/pritunl-client_${version}-0ubuntu1.jammy_amd64.deb"
  ["pkg-zip"]="${base_url}/Pritunl.pkg.zip"
  ["exe"]="${base_url}/Pritunl.exe"
)

digests=()
for key in deb-noble-amd64 deb-jammy-amd64 pkg-zip exe; do
  url="${artifacts[$key]}"
  file="${tmp}/${key}"
  echo ">> downloading ${url}"
  curl -fsSL "${url}" -o "${file}"
  # Guard against proxy/cdn error pages being saved as artifacts.
  if ! file "${file}" | grep -qiE 'debian binary package|zip archive|PE32|MS Windows|Zip archive'; then
    echo "error: ${key} does not look like a valid artifact:" >&2
    file "${file}" >&2
    exit 1
  fi
  digest="$(shasum -a 256 "${file}" | awk '{print $1}')"
  digests+=("${key}:${digest}")
  echo "   ${key} ${digest}"
done

python3 - "$manifest" "$version" "${digests[@]}" <<'PYEOF'
import json
import sys

manifest_path, version, *pairs = sys.argv[1:]
with open(manifest_path) as f:
    manifest = json.load(f)

manifest["artifacts"][version] = {key: digest for key, digest in
                                  (pair.split(":", 1) for pair in pairs)}
# stable ordering: numeric sort of versions, stable key order
ordered = {}
for v in sorted(manifest["artifacts"], key=lambda s: [int(p) for p in s.split(".")]):
    entry = manifest["artifacts"][v]
    ordered[v] = {k: entry[k] for k in sorted(entry)}

manifest["artifacts"] = ordered
with open(manifest_path, "w") as f:
    json.dump(manifest, f, indent=2)
    f.write("\n")
print(f"manifest updated for {version}")
PYEOF

echo ">> diff:"
git -C "$repo_root" diff --stat -- pkg/checksum/manifest.json
echo "next: go test ./pkg/checksum/ && commit pkg/checksum/manifest.json"