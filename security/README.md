# Pritunl Client Artifact Checksums

SHA-256 checksums of the official Pritunl Client release artifacts for the
versions this action supports with a pinned install. The manifest is embedded
into the action binary (`pkg/checksum/manifest.json` via go:embed); before any
versioned artifact is executed (`apt-get install <deb>`, macOS `installer`,
Windows installer) the downloaded file must match the checksum recorded here.

## Adding a version

1. Download the artifacts from the official release page:
   https://github.com/pritunl/pritunl-client-electron/releases
2. Compute SHA-256 for each artifact (`shasum -a 256 <file>`).
3. Update `manifest.json` with an entry keyed by the plain version number
   (no `v` prefix) and the artifact keys below.
4. Open a PR. CI unit tests assert the manifest parses and every listed
   checksum is well-formed.

## Artifact keys

| Key | Platform | File |
|---|---|---|
| `deb-noble-amd64` | Linux amd64 (Ubuntu 24.04) | `pritunl-client_<ver>-0ubuntu1.noble_amd64.deb` |
| `deb-jammy-amd64` | Linux amd64 (Ubuntu 22.04) | `pritunl-client_<ver>-0ubuntu1.jammy_amd64.deb` |
| `pkg-zip` | macOS | `Pritunl.pkg.zip` |
| `exe` | Windows | `Pritunl.exe` |

Versions not listed here are still installable when the caller supplies the
expected digest through the `client-sha256` input. For versions pinned here,
the manifest digest always wins: a `client-sha256` that differs from the
pinned digest is rejected.

## Verification policy

- Known version + manifest entry: checksum mandatory, must match.
- Known version + no manifest entry: rejected. Add the version first or use
  `client-sha256`.
- Unknown version + `client-sha256` set: the caller-provided checksum is used.
- Unknown version + no checksum: installation refused.
