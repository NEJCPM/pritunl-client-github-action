package provisioner

import (
	"context"
	"fmt"
	"os"

	"github.com/NEJCPM/pritunl-client-github-action/pkg/domain"
)

// WindowsProvisioner installs the Pritunl client on Windows runners via
// Chocolatey or a versioned installer download. Command execution is an
// injectable seam.
type WindowsProvisioner struct {
	run commandRunner
}

// NewWindowsProvisioner returns a WindowsProvisioner wired to the real system.
func NewWindowsProvisioner() *WindowsProvisioner {
	return &WindowsProvisioner{run: runCmd}
}

func (w *WindowsProvisioner) Provision(ctx context.Context, cfg domain.ActionConfig) error {
	if cfg.VPNMode == "wg" {
		if err := w.run(ctx, "choco", "install", "--no-progress", "-y", "wireguard"); err != nil {
			return fmt.Errorf("failed to install wireguard via choco: %w", err)
		}
	}

	if cfg.ClientVersion == "" || cfg.ClientVersion == "from-package-manager" {
		if err := w.run(ctx, "choco", "install", "--no-progress", "-y", "pritunl-client"); err != nil {
			return fmt.Errorf("failed to install pritunl-client via choco: %w", err)
		}
	} else {
		exeURL := fmt.Sprintf("https://github.com/pritunl/pritunl-client-electron/releases/download/%s/Pritunl.exe", cfg.ClientVersion)
		exeFile, err := createTempFile(getTempDir(cfg), "Pritunl-*.exe")
		if err != nil {
			return fmt.Errorf("failed to create temp file for Pritunl.exe: %w", err)
		}

		if err := w.run(ctx, "curl", "-fsSL", exeURL, "-o", exeFile); err != nil {
			return fmt.Errorf("failed to download Pritunl.exe from %s: %w", exeURL, err)
		}
		defer os.Remove(exeFile)

		psCmd := fmt.Sprintf("Start-Process -FilePath '%s' -ArgumentList '/VERYSILENT /SUPPRESSMSGBOXES /NORESTART /SP-' -Wait", exeFile)
		if err := w.run(ctx, "pwsh", "-ExecutionPolicy", "Bypass", "-Command", psCmd); err != nil {
			return fmt.Errorf("failed to run Pritunl installer: %w", err)
		}
	}

	return nil
}
