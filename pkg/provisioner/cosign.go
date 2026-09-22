package provisioner

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/NEJCPM/pritunl-client-github-action/pkg/domain"
)

// Cosign image verification policy (keyless, GitHub OIDC):
// only signatures produced by this repository's official image build
// workflow are trusted. Missing, invalid or foreign signatures fail the
// provisioning with no fallback.
const (
	cosignOIDCIssuer = "https://token.actions.githubusercontent.com"
	// cosignIdentityRegexp accepts the image build workflow triggered from
	// the main branch or from a release tag of this repository.
	cosignIdentityRegexp = `^https://github\.com/NEJCPM/pritunl-client-github-action/\.github/workflows/build-pritunl-image\.yml@refs/(heads/main|tags/.+)$`

	cosignVersion    = "v2.6.1"
	cosignInstallDir = "/usr/local/bin"

	// Pinned SHA-256 digests of the official cosign release binaries.
	cosignSHAAMD64 = "064954c5d8c7e3b28188eee5b1727b31c411550bc5fefd41aa672d3c761d103a"
	cosignSHAArm64 = "56a16480bdd56ec789abaa65924402f6b92c0041f06885995853c05567b76f34"
)

func cosignBinaryURL(goarch string) (string, string, error) {
	switch goarch {
	case "amd64":
		return fmt.Sprintf("https://github.com/sigstore/cosign/releases/download/%s/cosign-linux-amd64", cosignVersion), cosignSHAAMD64, nil
	case "arm64":
		return fmt.Sprintf("https://github.com/sigstore/cosign/releases/download/%s/cosign-linux-arm64", cosignVersion), cosignSHAArm64, nil
	default:
		return "", "", fmt.Errorf("unsupported architecture for cosign install: %s", goarch)
	}
}

// resolveImageDigest returns the remote digest of an already pulled image,
// as "repo@sha256:...".
func (l *LinuxProvisioner) resolveImageDigest(ctx context.Context, image string) (string, error) {
	out, err := l.runOutput(ctx, "docker", "image", "inspect", "--format", "{{index .RepoDigests 0}}", image)
	if err != nil {
		return "", fmt.Errorf("failed to resolve digest for %s: %w (output: %s)", image, err, strings.TrimSpace(out))
	}
	digest := strings.TrimSpace(out)
	if !strings.Contains(digest, "sha256:") {
		return "", fmt.Errorf("unexpected digest format for %s: %q", image, digest)
	}
	return digest, nil
}

// verifyImageSignature runs cosign verify against the image digest using the
// keyless GitHub OIDC policy. Fails closed.
func (l *LinuxProvisioner) verifyImageSignature(ctx context.Context, cfg domain.ActionConfig, digest string) error {
	if !strings.Contains(digest, "sha256:") {
		return fmt.Errorf("refusing to verify malformed image digest %q", digest)
	}
	if err := l.ensureCosign(ctx, cfg); err != nil {
		return fmt.Errorf("cosign is required to verify the image signature: %w", err)
	}

	err := l.run(ctx, "cosign", "verify",
		"--certificate-oidc-issuer", cosignOIDCIssuer,
		"--certificate-identity-regexp", cosignIdentityRegexp,
		digest,
	)
	if err != nil {
		return fmt.Errorf("image signature verification failed for %s: %w", digest, err)
	}
	return nil
}

// ensureCosign installs the pinned cosign binary when it is not on PATH.
func (l *LinuxProvisioner) ensureCosign(ctx context.Context, cfg domain.ActionConfig) error {
	if _, err := l.lookPath("cosign"); err == nil {
		return nil
	}

	url, expectedSHA, err := cosignBinaryURL(runtime.GOARCH)
	if err != nil {
		return err
	}

	tempFile, err := createTempFile(getTempDir(cfg), "cosign-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file for cosign: %w", err)
	}
	defer os.Remove(tempFile)

	if err := l.run(ctx, "curl", "-fsSL", url, "-o", tempFile); err != nil {
		return fmt.Errorf("failed to download cosign %s: %w", cosignVersion, err)
	}
	if err := l.cosignChecksum(tempFile, expectedSHA); err != nil {
		return fmt.Errorf("cosign download failed integrity check: %w", err)
	}
	if err := l.run(ctx, "sudo", "install", "-m", "0755", tempFile, cosignInstallDir+"/cosign"); err != nil {
		return fmt.Errorf("failed to install cosign: %w", err)
	}
	return nil
}
