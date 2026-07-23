package provisioner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nathanielvarona/pritunl-client-github-action/pkg/domain"
)

type WindowsProvisioner struct{}

func (w *WindowsProvisioner) Provision(ctx context.Context, cfg domain.ActionConfig) error {
	if cfg.VPNMode == "wg" {
		_ = runCmd(ctx, "choco", "install", "--no-progress", "-y", "wireguard")
	}

	if cfg.ClientVersion == "" || cfg.ClientVersion == "from-package-manager" {
		if err := runCmd(ctx, "choco", "install", "--no-progress", "-y", "pritunl-client"); err != nil {
			return fmt.Errorf("failed to install pritunl-client via choco: %w", err)
		}
	} else {
		exeURL := fmt.Sprintf("https://github.com/pritunl/pritunl-client-electron/releases/download/%s/Pritunl.exe", cfg.ClientVersion)
		exeFile := filepath.Join(getTempDir(cfg), "Pritunl.exe")

		if err := runCmd(ctx, "curl", "-sSL", exeURL, "-o", exeFile); err != nil {
			return fmt.Errorf("failed to download Pritunl.exe from %s: %w", exeURL, err)
		}
		defer os.Remove(exeFile)

		psCmd := fmt.Sprintf("Start-Process -FilePath '%s' -ArgumentList '/VERYSILENT /SUPPRESSMSGBOXES /NORESTART /SP-' -Wait", exeFile)
		if err := runCmd(ctx, "pwsh", "-ExecutionPolicy", "Bypass", "-Command", psCmd); err != nil {
			return fmt.Errorf("failed to run Pritunl installer: %w", err)
		}
	}

	return nil
}
