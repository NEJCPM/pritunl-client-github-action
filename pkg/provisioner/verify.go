package provisioner

import (
	"github.com/NEJCPM/pritunl-client-github-action/pkg/checksum"
	"github.com/NEJCPM/pritunl-client-github-action/pkg/domain"
)

// artifactVerifier resolves the expected SHA-256 digest for a downloaded
// artifact (embedded manifest first, caller-supplied client-sha256 as an
// override/escape hatch) and fails the install when it does not match.
type artifactVerifier func(cfg domain.ActionConfig, artifactKey string, filePath string) error

func newArtifactVerifier() artifactVerifier {
	return func(cfg domain.ActionConfig, artifactKey string, filePath string) error {
		manifest, err := checksum.Load()
		if err != nil {
			return err
		}
		expected, err := manifest.Expected(cfg.ClientVersion, artifactKey, cfg.ClientSHA256)
		if err != nil {
			return err
		}
		return checksum.VerifyFile(filePath, expected)
	}
}
