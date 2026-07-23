package provisioner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nathanielvarona/pritunl-client-github-action/pkg/domain"
)

type MacOSProvisioner struct{}

func (m *MacOSProvisioner) Provision(ctx context.Context, cfg domain.ActionConfig) error {
	if cfg.VPNMode == "wg" {
		_ = runCmd(ctx, "brew", "install", "-q", "wireguard-tools")
	}

	if cfg.ClientVersion == "" || cfg.ClientVersion == "from-package-manager" {
		if err := runCmd(ctx, "brew", "install", "-q", "--cask", "pritunl"); err != nil {
			return fmt.Errorf("failed to install pritunl cask via brew: %w", err)
		}
	} else {
		pkgZipURL := fmt.Sprintf("https://github.com/pritunl/pritunl-client-electron/releases/download/%s/Pritunl.pkg.zip", cfg.ClientVersion)
		zipFile := filepath.Join(getTempDir(cfg), "Pritunl.pkg.zip")

		if err := runCmd(ctx, "curl", "-sSL", pkgZipURL, "-o", zipFile); err != nil {
			return fmt.Errorf("failed to download macOS pkg zip from %s: %w", pkgZipURL, err)
		}
		defer os.Remove(zipFile)

		if err := runCmd(ctx, "unzip", "-qq", "-o", zipFile, "-d", getTempDir(cfg)); err != nil {
			return fmt.Errorf("failed to unzip %s: %w", zipFile, err)
		}

		pkgFile := filepath.Join(getTempDir(cfg), "Pritunl.pkg")
		defer os.Remove(pkgFile)

		if err := runCmd(ctx, "sudo", "installer", "-pkg", pkgFile, "-target", "/"); err != nil {
			return fmt.Errorf("failed to install macOS pkg %s: %w", pkgFile, err)
		}
	}

	// Symlink binary to user bin directory
	clientBin := "/Applications/Pritunl.app/Contents/Resources/pritunl-client"
	userBinDir := filepath.Join(os.Getenv("HOME"), "bin")
	_ = os.MkdirAll(userBinDir, 0755)
	targetSymlink := filepath.Join(userBinDir, "pritunl-client")

	_ = os.Remove(targetSymlink)
	if err := os.Symlink(clientBin, targetSymlink); err != nil {
		fmt.Printf("Warning: failed to create symlink at %s: %v\n", targetSymlink, err)
	}

	return nil
}
