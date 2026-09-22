// Package checksum verifies downloaded Pritunl Client artifacts against
// SHA-256 digests pinned in the embedded security manifest (or supplied by
// the caller for versions that are not pinned).
package checksum

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"os"
	"regexp"
)

//go:embed manifest.json
var manifestFS embed.FS

var sha256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Manifest holds the pinned artifact digests, keyed by version and artifact
// key (see security/README.md for the key table).
type Manifest struct {
	Schema    int                          `json:"schema"`
	Artifacts map[string]map[string]string `json:"artifacts"`
}

// Load reads the embedded manifest.
func Load() (*Manifest, error) {
	data, err := manifestFS.ReadFile("manifest.json")
	if err != nil {
		return nil, fmt.Errorf("failed to read embedded checksum manifest: %w", err)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("failed to parse embedded checksum manifest: %w", err)
	}
	if m.Schema != 1 {
		return nil, fmt.Errorf("unsupported checksum manifest schema %d", m.Schema)
	}
	return &m, nil
}

// Expected returns the digest that must match the artifact.
//
// Policy:
//   - Pinned version with a manifest digest: the manifest wins. A caller
//     input that differs is rejected - pinned digests cannot be overridden.
//   - Pinned version without a digest for the key: rejected.
//   - Unpinned version: requires the caller-supplied client-sha256 input.
func (m *Manifest) Expected(version, artifactKey, clientSHA256 string) (string, error) {
	normalized := normalizeSHA256(clientSHA256)

	entry, pinned := m.Artifacts[version]
	var pinnedDigest string
	if pinned {
		pinnedDigest = entry[artifactKey]
	}

	if pinned {
		if pinnedDigest == "" {
			return "", fmt.Errorf("version %s is pinned in the checksum manifest but has no digest for %q; update pkg/checksum/manifest.json", version, artifactKey)
		}
		if normalized != "" && normalized != pinnedDigest {
			return "", fmt.Errorf("client-sha256 does not match the pinned digest for version %s (%q); pinned digests cannot be overridden", version, artifactKey)
		}
		return pinnedDigest, nil
	}

	if normalized == "" {
		return "", fmt.Errorf("version %s has no pinned checksum for %q; add it to pkg/checksum/manifest.json or supply client-sha256", version, artifactKey)
	}
	return normalized, nil
}

// VerifyFile hashes the file at path and compares it with the expected
// SHA-256 digest (case-insensitive, 64 hex chars).
func VerifyFile(path, expected string) error {
	normalized := normalizeSHA256(expected)
	if normalized == "" {
		return fmt.Errorf("invalid expected sha256 %q", expected)
	}

	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open artifact %s: %w", path, err)
	}
	defer f.Close()

	var hasher hash.Hash = sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return fmt.Errorf("failed to hash artifact %s: %w", path, err)
	}

	actual := hex.EncodeToString(hasher.Sum(nil))
	if actual != normalized {
		return fmt.Errorf("checksum mismatch for %s: expected %s, got %s", path, normalized, actual)
	}
	return nil
}

func normalizeSHA256(val string) string {
	val = lowerTrim(val)
	if !sha256Pattern.MatchString(val) {
		return ""
	}
	return val
}

func lowerTrim(val string) string {
	out := make([]byte, 0, len(val))
	for i := 0; i < len(val); i++ {
		c := val[i]
		if c == ' ' {
			continue
		}
		if c >= 'A' && c <= 'F' {
			c += 32
		}
		out = append(out, c)
	}
	return string(out)
}
